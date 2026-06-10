package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents an active browser connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan LiveMetrics
	ip   string
}

// SubscriptionMessage maps the incoming client subscription handshake
type SubscriptionMessage struct {
	Action       string `json:"action"`
	SubmissionID string `json:"submission_id"`
}

// Hub manages WebSocket registrations, unregistrations, routing, and caching
type Hub struct {
	// Registered clients by submission ID
	subscriptions map[string]map[*Client]bool

	// Late-Joiner Cache: Stores the most recent metric frame per submission ID
	cache   map[string]LiveMetrics
	cacheMu sync.RWMutex

	// Inbound infrastructure channels
	broadcast  chan LiveMetrics
	register   chan *Client
	unregister chan *Client

	// Rate limiting state
	ipRates map[string]*TokenBucket
	rateMu  sync.Mutex
}

// TokenBucket handles IP-based connection rate limiting
type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func NewHub() *Hub {
	return &Hub{
		subscriptions: make(map[string]map[*Client]bool),
		cache:         make(map[string]LiveMetrics),
		broadcast:     make(chan LiveMetrics, 10000),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		ipRates:       make(map[string]*TokenBucket),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// Client connection is tracked, waiting for explicit subscription frame
			log.Printf("[Hub] Client connected from IP: %s", client.ip)

		case client := <-h.unregister:
			h.cleanupClient(client)

		case metrics := <-h.broadcast:
			// Update Late-Joiner Cache
			h.cacheMu.Lock()
			h.cache[metrics.SubmissionID] = metrics
			h.cacheMu.Unlock()

			// Route only to clients subscribed to this submission
			if clients, exists := h.subscriptions[metrics.SubmissionID]; exists {
				for client := range clients {
					select {
					case client.send <- metrics:
					default:
						h.cleanupClient(client)
					}
				}
			}
		}
	}
}

func (h *Hub) cleanupClient(client *Client) {
	// Remove client from any active subscription groups
	for subID, clients := range h.subscriptions {
		if _, exists := clients[client]; exists {
			delete(clients, client)
			log.Printf("[Hub] Unsubscribed client from run: %s", subID)
			if len(clients) == 0 {
				delete(h.subscriptions, subID)
			}
		}
	}
	close(client.send)
	client.conn.Close()
}

// AllowIP checks if an incoming IP address has tokens available to establish a connection
func (h *Hub) AllowIP(ip string) bool {
	h.rateMu.Lock()
	defer h.rateMu.Unlock()

	bucket, exists := h.ipRates[ip]
	if !exists {
		bucket = &TokenBucket{
			tokens:     5, // Start with burst capacity
			maxTokens:  5,
			refillRate: 1.0, // Refill 1 token per second
			lastRefill: time.Now(),
		}
		h.ipRates[ip] = bucket
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * bucket.refillRate
	if bucket.tokens > bucket.maxTokens {
		bucket.tokens = bucket.maxTokens
	}
	bucket.lastRefill = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}
	return false
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
	}()

	// Configure strict read thresholds to mitigate Slowloris / stalled socket attacks
	c.conn.SetReadLimit(512) // Subscriptions are tiny text payloads
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg SubscriptionMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msg.Action == "subscribe" && msg.SubmissionID != "" {
			c.hub.cacheMu.RLock()
			cachedFrame, hasCache := c.hub.cache[msg.SubmissionID]
			c.hub.cacheMu.RUnlock()

			// Immediately flush cached layout frame if a late joiner connects
			if hasCache {
				c.conn.WriteJSON(cachedFrame)
			}

			// Assign client to the exact telemetry room
			if c.hub.subscriptions[msg.SubmissionID] == nil {
				c.hub.subscriptions[msg.SubmissionID] = make(map[*Client]bool)
			}
			c.hub.subscriptions[msg.SubmissionID][c] = true
			log.Printf("[Hub] Client explicitly subscribed to submission: %s", msg.SubmissionID)
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case metrics, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(metrics); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
