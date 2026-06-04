import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

export async function POST(request: Request) {
  try {
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
      `[Next.js API] Injecting payload to Orchestrator for Submission: ${submission.id}`,
    );

    const orchestratorRes = await fetch("http://localhost:8080/deploy", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },

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
