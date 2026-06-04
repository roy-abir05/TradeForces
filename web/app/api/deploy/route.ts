import { NextResponse } from "next/server";
import { writeFile, mkdir } from "fs/promises";
import path from "path";

export async function POST(request: Request) {
  try {
    const data = await request.formData();
    const file: File | null = data.get("engine") as unknown as File;

    if (!file) {
      return NextResponse.json(
        { success: false, error: "No file found" },
        { status: 400 },
      );
    }

    const bytes = await file.arrayBuffer();
    const buffer = Buffer.from(bytes);

    const submissionDir = path.join(
      process.cwd(),
      "..",
      "orchestrator",
      "submissions",
      "user_123",
    );
    await mkdir(submissionDir, { recursive: true });

    const filePath = path.join(submissionDir, "server.cpp");

    await writeFile(filePath, buffer);
    console.log(`[API] Successfully wrote submission to ${filePath}`);

    console.log("[Next.js API] Signaling Orchestrator to boot...");
    const orchestratorRes = await fetch("http://localhost:8080/deploy", {
      method: "POST",
    });

    if (!orchestratorRes.ok) {
        throw new Error("Go Orchestrator failed to boot container");
    }

    return NextResponse.json({
      success: true,
      message: "Engine uploaded to Sandbox",
    });
  } catch (error) {
    console.error("[API] Upload failed:", error);
    return NextResponse.json(
      { success: false, error: "Server upload failed" },
      { status: 500 },
    );
  }
}
