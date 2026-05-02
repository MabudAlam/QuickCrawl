import { NextResponse } from "next/server";
import fs from "fs";
import path from "path";

const BENCH_DIR = "/Users/skmabudalam/Desktop/gocrawl/bench";

export async function GET() {
  try {
    const files = fs.readdirSync(BENCH_DIR)
      .filter((f) => f.startsWith("flush_") && f.endsWith(".json"))
      .sort()
      .reverse();
    return NextResponse.json(files);
  } catch (e) {
    console.error("Failed to read flush files:", e);
    return NextResponse.json([], { status: 500 });
  }
}
