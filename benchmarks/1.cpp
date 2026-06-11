// bad_engine.cpp
#include <iostream>
#include <string>
#include <thread>
#include <mutex>
#include <vector>
#include <netinet/in.h>
#include <unistd.h>
#include <cstring>

std::mutex book_mutex;
std::vector<std::string> order_book;

void handle_connection(int client_sock) {
    char buffer[256] = {0};
    read(client_sock, buffer, 255);
    std::string order(buffer);

    // THE BOTTLENECK: Global lock + string copies + O(N) linear search
    std::lock_guard<std::mutex> lock(book_mutex);
    order_book.push_back(order);

    int dummy_matches = 0;
    for (const auto& o : order_book) {
        if (o.find("BUY") != std::string::npos && order.find("SELL") != std::string::npos) {
            dummy_matches++;
        }
    }

    std::string response = "ACK " + std::to_string(dummy_matches) + "\n";
    send(client_sock, response.c_str(), response.length(), 0);
    close(client_sock);
}

int main() {
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(1337);

    bind(server_fd, (struct sockaddr*)&address, sizeof(address));
    listen(server_fd, 1000);

    std::cout << "[Bad Engine] Listening on 1337. Prepare for mutex contention..." << std::endl;

    while (true) {
        int client_sock = accept(server_fd, nullptr, nullptr);
        // THE BOTTLENECK: Spawning a heavy OS thread per connection
        std::thread(handle_connection, client_sock).detach();
    }
    return 0;
}
