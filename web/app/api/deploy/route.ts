import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

const MAX_FILE_SIZE = 500 * 1024; // 500KB

interface RateLimitData {
  attempts: number;
  windowStart: number;
  penaltyUntil: number;
}
const rateLimitMap = new Map<string, RateLimitData>();

const MAX_ATTEMPTS = 10; // Allow 5 submissions...
const WINDOW_MS = 60 * 1000; // ...within any 1-minute window
const PENALTY_MS = 5 * 60 * 1000; // 5-minute ban if they spam

export async function POST(request: Request) {
  try {
    // Rate Limiter
    const userId = request.headers.get("x-user-id");

    if (!userId) {
      return NextResponse.json(
        { success: false, error: "Missing x-user-id header" },
        { status: 400 },
      );
    }

    const now = Date.now();
    const userStats = rateLimitMap.get(userId) || {
      attempts: 0,
      windowStart: now,
      penaltyUntil: 0,
    };

    if (now < userStats.penaltyUntil) {
      const remainingMinutes = Math.ceil(
        (userStats.penaltyUntil - now) / 60000,
      );
      console.warn(`[Firewall] Blocked penalized user: ${userId}`);
      return NextResponse.json(
        {
          success: false,
          error: `Spam detected. Banned for ${remainingMinutes} minute(s).`,
        },
        { status: 429 },
      );
    }

    if (now - userStats.windowStart > WINDOW_MS) {
      userStats.attempts = 0;
      userStats.windowStart = now;
    }

    userStats.attempts += 1;

    if (userStats.attempts > MAX_ATTEMPTS) {
      userStats.penaltyUntil = now + PENALTY_MS;
      rateLimitMap.set(userId, userStats);
      console.warn(
        `[Firewall] User ${userId} spamming. Sent to 5-minute penalty box.`,
      );
      return NextResponse.json(
        {
          success: false,
          error: "Too many requests. You are blocked for 5 minutes.",
        },
        { status: 429 },
      );
    }

    rateLimitMap.set(userId, userStats);

    const contentLengthHeader = request.headers.get("content-length");
    if (contentLengthHeader) {
      const contentLength = parseInt(contentLengthHeader, 10);
      if (contentLength > MAX_FILE_SIZE) {
        console.warn(
          `[Firewall] Blocked via Content-Length header: ${contentLength} bytes`,
        );
        return NextResponse.json(
          { success: false, error: "Payload too large (Header Gate)" },
          { status: 413 },
        );
      }
    }

    const streamClone = request.clone();
    const reader = streamClone.body?.getReader();

    let totalBytesRead = 0;

    if (reader) {
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          if (value) {
            totalBytesRead += value.length;
          }

          if (totalBytesRead > MAX_FILE_SIZE) {
            console.warn(
              `[Firewall] Circuit breaker tripped! Stream exceeded 500KB. Aborting connection.`,
            );

            await reader.cancel();
            return NextResponse.json(
              {
                success: false,
                error: "Payload too large (Stream Circuit Breaker)",
              },
              { status: 413 },
            );
          }
        }
      } catch (streamError) {
        console.error(
          "[Firewall] Error during stream inspection:",
          streamError,
        );
        return NextResponse.json(
          { success: false, error: "Stream inspection failed" },
          { status: 400 },
        );
      }
    }

    const data = await request.formData();
    const file: File | null = data.get("engine") as unknown as File;

    if (!file) {
      return NextResponse.json(
        { success: false, error: "Missing file" },
        { status: 400 },
      );
    }

    const codeString = await file.text();

    const submission = await prisma.submission.create({
      data: {
        userId: userId,
        status: "PENDING",
        filePath: "IN_MEMORY",
      },
    });

    console.log(
      `[Next.js API] Injecting payload to Orchestrator: ${submission.id}`,
    );

    const orchestratorRes = await fetch("http://localhost:8080/deploy", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        submissionId: submission.id,
        code: codeString,
      }),
    });

    if (!orchestratorRes.ok) {
      await prisma.submission.update({
        where: { id: submission.id },
        data: { status: "FAILED" },
      });
      // Revert lock on orchestrator failure
      userStats.attempts = Math.max(0, userStats.attempts - 1);
      rateLimitMap.set(userId, userStats);
      throw new Error("Go Orchestrator failed to boot container");
    }

    return NextResponse.json({
      success: true,
      message: "Engine payload injected to Sandbox",
      submissionId: submission.id,
    });
  } catch (error) {
    console.error("[API] Upload failed:", error);
    return NextResponse.json(
      { success: false, error: "Server upload failed" },
      { status: 500 },
    );
  }
}
