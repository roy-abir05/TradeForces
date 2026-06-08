import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

export async function POST(request: Request) {
  try {
    const body = await request.json();

    const { submissionId, status, tps, p50, p90, p99, cv } = body;

    if (!submissionId) {
      return NextResponse.json(
        { error: "Missing submissionId" },
        { status: 400 },
      );
    }

    if (status !== "SUCCESS") {
      await prisma.submission.update({
        where: { id: submissionId },
        data: { status: status },
      });
      return NextResponse.json({ success: true });
    }

    await prisma.result.create({
      data: {
        submissionId,
        tps: Number(tps) || 0,
        p50: Number(p50) || 0,
        p90: Number(p90) || 0,
        p99: Number(p99) || 0,
        cv: Number(cv) || 0,
      },
    });

    await prisma.submission.update({
      where: { id: submissionId },
      data: { status: "SUCCESS" },
    });

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error(error);
    return NextResponse.json(
      { error: "Failed to save result" },
      { status: 500 },
    );
  }
}
