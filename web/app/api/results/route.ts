import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

declare global {
  var _cachedMetrics: unknown;
}

interface Metrics {
  tps: number;
  p50: number;
  p90: number;
  p99: number;
}

export async function POST(request: Request) {
  try {
    const body = await request.json();
    const { submissionId, status } = body;

    if (!submissionId) {
      return NextResponse.json(
        { error: "Missing submissionId" },
        { status: 400 },
      );
    }

    if (status === "FAILED") {
      await prisma.submission.update({
        where: { id: submissionId },
        data: { status: "FAILED" },
      });
      return NextResponse.json({ success: true });
    }

    const metrics = globalThis._cachedMetrics as Metrics;
    const finalMetrics = metrics || { tps: 0, p50: 0, p90: 0, p99: 0 };

    await prisma.result.create({
      data: {
        submissionId,
        tps: Number(finalMetrics.tps),
        p50: Number(finalMetrics.p50),
        p90: Number(finalMetrics.p90),
        p99: Number(finalMetrics.p99),
      },
    });

    await prisma.submission.update({
      where: { id: submissionId },
      data: { status: "SUCCESS" },
    });

    globalThis._cachedMetrics = { tps: 0, p50: 0, p90: 0, p99: 0 };

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error(error);
    return NextResponse.json(
      { error: "Failed to save result" },
      { status: 500 },
    );
  }
}
