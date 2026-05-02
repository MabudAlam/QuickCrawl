import PlaygroundPage from "./page";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Playground - quickcrawl",
};

export default async function PlaygroundLayout() {
  const baseUrl = process.env.NEXT_PUBLIC_BASE_URL;

  return <PlaygroundPage initialBaseUrl={baseUrl} />;
}