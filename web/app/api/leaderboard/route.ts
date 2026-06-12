import { NextResponse } from "next/server";
import { prisma } from "../../../lib/prisma";

export const revalidate = 10;

export async function GET() {
  try {
    const topResults = await prisma.result.findMany({
      take: 10,
      orderBy: [{ tps: "desc" }, { p99: "asc" }],
      include: {
        submission: {
          select: {
            user: {
              select: { name: true, image: true },
            },
          },
        },
      },
    });

    return NextResponse.json(topResults);
  } catch (error) {
    console.error("[API] Leaderboard fetch failed:", error);
    return NextResponse.json(
      { error: "Failed to fetch leaderboard" },
      { status: 500 },
    );
  }
}
