import { NextResponse } from "next/server";
import fs from "fs";
import path from "path";

const BENCH_DIR = "/Users/skmabudalam/Desktop/gocrawl/bench";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const file = searchParams.get("file");

  if (!file) {
    return NextResponse.json({ error: "Missing file parameter" }, { status: 400 });
  }

  const filePath = path.join(BENCH_DIR, file);

  try {
    const content = fs.readFileSync(filePath, "utf-8");
    const data = JSON.parse(content);
    return NextResponse.json(data);
  } catch (e) {
    console.error("Failed to read flush file:", e);
    return NextResponse.json({ error: "Failed to read file" }, { status: 500 });
  }
}
