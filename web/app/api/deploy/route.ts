import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

const MAX_FILE_SIZE = 500 * 1024; // 500KB

export async function POST(request: Request) {
  try {
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

    // File Size Limit Check
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
    const userId = data.get("userId") as string;

    if (!file || !userId) {
      return NextResponse.json(
        { success: false, error: "Missing file or user ID" },
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
