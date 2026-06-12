import { Readability } from "@mozilla/readability";
import { JSDOM } from "jsdom";
import { readFileSync, writeFileSync } from "fs";

const inputPath = process.argv[2] || "internal/extractor/raw.html";
const outputPath = process.argv[3] || "internal/extractor/clean.html";

const html = readFileSync(inputPath, "utf-8");
const doc = new JSDOM(html, { url: "https://example.com" });
const reader = new Readability(doc.window.document);
const article = reader.parse();

if (article) {
  console.log("Title:", article.title);
  console.log("Byline:", article.byline);
  console.log("Excerpt:", article.excerpt);
  console.log("Site Name:", article.siteName);
  console.log("Length:", article.length);
  console.log("\n--- Content ---\n");
  console.log(article.content);
  writeFileSync(outputPath, article.content, "utf-8");
  console.log(`\nSaved clean HTML to: ${outputPath}`);
} else {
  console.error("Failed to parse article");
  process.exit(1);
}

//node scripts/parse-readability.mjs internal/extractor/raw.html internal/extractor/clean.html
