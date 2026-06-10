"use client"

import { useState, useMemo } from "react"
import {
  Loader2,
  Play,
  AlertCircle,
  CheckCircle,
  XCircle,
  LayoutDashboard,
  ChevronDown,
  SunIcon,
  MoonIcon,
  BarChart3,
  Eye,
  ExternalLink,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioItem,
  DropdownMenuRadioGroup,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Progress } from "@/components/ui/progress"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useTheme } from "next-themes"
import ReactMarkdown from "react-markdown"

function computeSimpleDiff(textA: string, textB: string): { a: string[]; b: string[] } {
  const wordsA = textA.split(/\s+/)
  const wordsB = textB.split(/\s+/)
  const resultA: string[] = []
  const resultB: string[] = []
  const setB = new Set(wordsB)

  for (const word of wordsA) {
    if (setB.has(word)) {
      resultA.push(word)
    } else {
      resultA.push(`[[${word}]]`)
    }
  }

  const setA = new Set(wordsA)
  for (const word of wordsB) {
    if (setA.has(word)) {
      resultB.push(word)
    } else {
      resultB.push(`[[${word}]]`)
    }
  }

  return { a: resultA, b: resultB }
}

const BATCH_1 = [
  {
    url: "https://www.dailymail.co.uk/news/article-15881367/ex-Nato-chief-chilling-warning-Starmer-spend-defence-Britain-cost-blood.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.dailymail.co.uk/news/article-15881685/8-2-earthquake-Philippines.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.dailymail.co.uk/news/article-15869813/Glamorous-sheep-farmer-faces-jail-secretly-building-second-home-inside-barn-40-acre-farm-despite-paying-council-tax.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.hindustantimes.com/india-news/3-reasons-why-vijay-led-tvk-will-not-be-a-part-of-todays-india-bloc-meeting-101780894736751.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.hindustantimes.com/cricket/ind-vs-afg-live-cricket-score-india-vs-afghanistan-one-off-test-match-day-3-2026-new-chandigarh-june-8-101780882653311.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.hindustantimes.com/trending/bengaluru-founder-says-finding-a-flat-was-a-nightmare-despite-80-000-monthly-budget-paid-5-lakh-deposit-101780890684540.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.scmp.com/news/china/military/article/3356269/china-adds-warheads-nuclear-powers-walk-away-disarmament-sipri",
    publisher: "SCMP",
  },
  {
    url: "https://www.scmp.com/native/business/topics/how-design-drives-global-growth/article/3353635/how-changan-automobiles-design-driven-brand-strategy-shapes-its-global-expansion-plans?module=top_story&pgtype=homepage",
    publisher: "SCMP",
  },
  {
    url: "https://www.scmp.com/news/china/science/article/3356271/tibet-quartz-discovery-boosts-chinas-self-sufficiency-push-hi-tech-materials?module=top_story&pgtype=homepage",
    publisher: "SCMP",
  },
  {
    url: "https://www.theguardian.com/travel/2026/jun/08/ireland-joyce-country-western-lakes-unesco-geopark-county-galway-mayo",
    publisher: "The Guardian",
  },
  {
    url: "https://www.theguardian.com/commentisfree/2026/jun/07/the-guardian-view-on-cancer-treatments-new-hope-for-patients-now-and-in-the-future",
    publisher: "The Guardian",
  },
  {
    url: "https://www.theguardian.com/commentisfree/2026/jun/07/the-guardian-view-on-the-french-presidential-election-campaign-only-the-far-right-will-profit-from-division",
    publisher: "The Guardian",
  },
  {
    url: "https://www.nytimes.com/2026/06/08/world/asia/north-korea-kim-jong-un-pandemic-economy.html",
    publisher: "NY Times",
  },
  {
    url: "https://www.nytimes.com/2026/06/05/dining/craft-restaurant-closing-tom-colicchio.html",
    publisher: "NY Times",
  },
  {
    url: "https://www.nytimes.com/2026/06/03/travel/uk-travelers-eta-outage.html",
    publisher: "NY Times",
  },
]

const BATCH_2 = [
  {
    url: "https://www.dailymail.co.uk/news/article-15789623/Obama-Trump-marriage-Michelle-issues.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.dailymail.co.uk/news/article-15791231/Meghan-Markle-habit-falling-Hollywoods-power-brokers-SNLs-terrorist-takedown-Met-Gala-absence-proof-duchess-burning-bridges-US-experts-say.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.dailymail.co.uk/news/article-15791581/canada-actress-claire-brosseau-assisted-suicide.html",
    publisher: "Daily Mail",
  },
  {
    url: "https://www.hindustantimes.com/cricket/axar-patel-comes-straight-to-the-point-name-drops-kuldeep-yadav-after-delhi-capitals-stand-on-cusp-of-elimination-101777998122728.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.hindustantimes.com/cricket/bcci-controls-the-icc-south-africa-spinner-makes-big-charge-reveals-how-narrative-can-be-changed-101777989002691.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.hindustantimes.com/cricket/exindia-cricketer-claims-tmc-denied-him-ticket-after-he-refused-to-pay-inr-5-crore-the-chapter-is-over-101777982797802.html",
    publisher: "Hindustan Times",
  },
  {
    url: "https://www.scmp.com/news/china/diplomacy/article/3352532/irans-top-diplomat-abbas-araghchi-visit-china-days-ahead-donald-trump",
    publisher: "SCMP",
  },
  {
    url: "https://www.scmp.com/news/china/article/3352539/panama-minister-blasts-chinas-ship-crackdown-tells-deputies-demand-answers-beijing",
    publisher: "SCMP",
  },
  {
    url: "https://www.scmp.com/tech/big-tech/article/3352200/supersized-and-scaling-china-pushes-10000-card-computing-clusters-ai-race",
    publisher: "SCMP",
  },
  {
    url: "https://www.theguardian.com/film/2026/may/04/blake-lively-justin-baldoni-settlement-it-ends-with-us",
    publisher: "The Guardian",
  },
  {
    url: "https://www.theguardian.com/lifeandstyle/2026/may/05/kitten-rescued-glue-bucket-texas-adoption",
    publisher: "The Guardian",
  },
  {
    url: "https://www.theguardian.com/sport/2026/may/05/liv-golf-funding-cam-smith-australian",
    publisher: "The Guardian",
  },
  {
    url: "https://www.nytimes.com/2026/04/28/opinion/ezra-klein-podcast-thompson-dunkelman.html",
    publisher: "NY Times",
  },
  {
    url: "https://www.nytimes.com/2026/04/29/technology/ai-artificial-intelligence-ad-boom.html",
    publisher: "NY Times",
  },
  {
    url: "https://www.nytimes.com/2026/04/29/technology/ai-spending-tech-data-centers.html",
    publisher: "NY Times",
  },
]

const BATCHES = { batch1: BATCH_1, batch2: BATCH_2 } as const
type BatchKey = keyof typeof BATCHES

interface ArticleResult {
  url: string
  publisher: string
  quickcrawl: {
    success: boolean
    charCount: number
    timeMs: number
    markdown?: string
    error?: string
  }
  tinyfish: {
    success: boolean
    charCount: number
    timeMs: number
    markdown?: string
    error?: string
  }
}

function getBaseUrl() {
  return process.env.NEXT_PUBLIC_BASE_URL
}

async function scrapeWithQuickCrawl(url: string, renderJs: boolean = true, ttl: number = 0): Promise<{ charCount: number; timeMs: number; success: boolean; markdown?: string; error?: string }> {
  const baseUrl = getBaseUrl()
  const startTime = Date.now()

  try {
    const response = await fetch(`${baseUrl}/v1/scrape`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url, formats: ["markdown"], renderJs, ttl }),
    })

    const timeMs = Date.now() - startTime
    const data = await response.json()

    if (!response.ok) {
      return { charCount: 0, timeMs, success: false, error: data.error || response.statusText }
    }

    const markdown = data.data?.markdown || ""
    return { charCount: markdown.length, timeMs, success: true, markdown }
  } catch (error) {
    return {
      charCount: 0,
      timeMs: Date.now() - startTime,
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

async function scrapeWithTinyFish(url: string): Promise<{ charCount: number; timeMs: number; success: boolean; markdown?: string; error?: string }> {
  const apiKey = process.env.NEXT_PUBLIC_TINY_FISH_API_KEY
  const startTime = Date.now()

  if (!apiKey) {
    return { charCount: 0, timeMs: Date.now() - startTime, success: false, error: "TINY_FISH_API_KEY not configured" }
  }

  try {
    const response = await fetch("https://api.fetch.tinyfish.ai", {
      method: "POST",
      headers: {
        "X-API-Key": apiKey,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ urls: [url], format: "markdown", ttl: 0 }),
    })

    const timeMs = Date.now() - startTime
    const data = await response.json()
    const result = data.results?.[0]

    if (!response.ok || !result || result.error) {
      return { charCount: 0, timeMs, success: false, error: result?.error || "No content" }
    }

    const markdown = result.text || ""
    return { charCount: markdown.length, timeMs, success: true, markdown }
  } catch (error) {
    return {
      charCount: 0,
      timeMs: Date.now() - startTime,
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export default function BattleArticlesPage() {
  const [isRunning, setIsRunning] = useState(false)
  const [results, setResults] = useState<ArticleResult[]>([])
  const [currentIndex, setCurrentIndex] = useState(-1)
  const [error, setError] = useState<string | null>(null)
  const [renderJs, setRenderJs] = useState<boolean | null>(null)
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)
  const [selectedResult, setSelectedResult] = useState<ArticleResult | null>(null)
  const [selectedBatch, setSelectedBatch] = useState<BatchKey>("batch1")

  const activeUrls = BATCHES[selectedBatch]

  

  const handleStart = async () => {
    setIsRunning(true)
    setError(null)
    setResults([])
    setCurrentIndex(-1)

    const newResults: ArticleResult[] = []

    for (let i = 0; i < activeUrls.length; i++) {
      const article = activeUrls[i]
      setCurrentIndex(i)

      const [qcResult, tfResult] = await Promise.all([
        scrapeWithQuickCrawl(article.url, true, 0),
        scrapeWithTinyFish(article.url),
      ])

      newResults.push({
        url: article.url,
        publisher: article.publisher,
        quickcrawl: qcResult,
        tinyfish: tfResult,
      })
      setResults([...newResults])
    }

    setCurrentIndex(-1)
    setIsRunning(false)
  }

  const totalQuickcrawl = results.reduce((sum, r) => sum + r.quickcrawl.charCount, 0)
  const totalTinyfish = results.reduce((sum, r) => sum + r.tinyfish.charCount, 0)
  const successCount = results.filter((r) => r.quickcrawl.success && r.tinyfish.success).length

  return (
    <div className="min-h-screen bg-background">
      <div className="container mx-auto max-w-7xl px-4 py-8">
        <div className="mb-8">
          <div className="mb-2 flex items-center justify-between">
            <h1 className="text-3xl font-bold tracking-tight">Battle Articles</h1>
            <div className="flex items-center gap-3">
              <a href="/battle">
                <Button variant="noShadow">
                  <BarChart3 className="h-4 w-4" />
                  Single URL
                </Button>
              </a>
              <a href="/playground">
                <Button variant="noShadow">
                  <LayoutDashboard className="h-4 w-4" />
                  Playground
                </Button>
              </a>
              {mounted && (
                <Button
                  variant="noShadow"
                  onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
                >
                  {resolvedTheme === "dark" ? <SunIcon className="h-4 w-4" /> : <MoonIcon className="h-4 w-4" />}
                </Button>
              )}
            </div>
          </div>
          <p className="text-muted-foreground">
            Compare QuickCrawl and TinyFish across {activeUrls.length} news articles
          </p>
        </div>

        <Card className="mb-8">
          <CardContent className="pt-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <div className="flex items-center gap-2">
                <label className="text-sm">Batch</label>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="default" size="sm" className="h-8 w-[120px] justify-start">
                      {selectedBatch === "batch1" ? "Batch 1" : "Batch 2"}
                      <ChevronDown className="ml-auto h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuRadioGroup
                      value={selectedBatch}
                      onValueChange={(v) => setSelectedBatch(v as BatchKey)}
                    >
                      <DropdownMenuRadioItem value="batch1">
                        Batch 1 ({BATCH_1.length} URLs)
                      </DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="batch2">
                        Batch 2 ({BATCH_2.length} URLs)
                      </DropdownMenuRadioItem>
                    </DropdownMenuRadioGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              <div className="flex items-center gap-2">
                <label className="text-sm">Renderer</label>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="default" size="sm" className="h-8 w-[140px] justify-start">
                      {renderJs === null ? "Auto" : renderJs ? "Browser" : "HTTP"}
                      <ChevronDown className="ml-auto h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuRadioGroup
                      value={renderJs === null ? "auto" : renderJs ? "browser" : "http"}
                      onValueChange={(v) => setRenderJs(v === "auto" ? null : v === "browser")}
                    >
                      <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="browser">Browser</DropdownMenuRadioItem>
                    </DropdownMenuRadioGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              <Button onClick={handleStart} disabled={isRunning} className="w-full sm:w-auto">
                {isRunning ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Running... {currentIndex >= 0 ? `${currentIndex + 1}/${activeUrls.length}` : ""}
                  </>
                ) : (
                  <>
                    <Play className="mr-2 h-4 w-4" />
                    Start Benchmark
                  </>
                )}
              </Button>

              {results.length > 0 && (
                <div className="flex items-center gap-4 text-sm">
                  <div className="flex items-center gap-2">
                    <div className="h-3 w-3 rounded-full bg-green-500" />
                    <span>QuickCrawl</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="h-3 w-3 rounded-full bg-blue-500" />
                    <span>TinyFish</span>
                  </div>
                </div>
              )}
            </div>

            {error && (
              <Alert variant="destructive" className="mt-4">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </CardContent>
        </Card>

        {isRunning && currentIndex >= 0 && (
          <Card className="mb-8">
            <CardContent className="pt-6">
              <div className="mb-2 flex items-center justify-between text-sm">
                <span>
                  Processing: {activeUrls[currentIndex]?.publisher || ""}...
                </span>
                <span>
                  {currentIndex + 1} / {activeUrls.length}
                </span>
              </div>
              <Progress value={((currentIndex + 1) / activeUrls.length) * 100} className="h-2" />
            </CardContent>
          </Card>
        )}

        {results.length > 0 && (
          <div className="space-y-6">
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Articles Tested</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{results.length}</div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Both Succeeded</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-green-500">
                    {successCount}/{results.length}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Total Chars (QC)</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-green-500">{totalQuickcrawl.toLocaleString()}</div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Total Chars (TF)</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-blue-500">{totalTinyfish.toLocaleString()}</div>
                </CardContent>
              </Card>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-border">
                    <th className="pb-3 text-left font-medium">Publisher</th>
                    <th className="pb-3 text-left font-medium">QuickCrawl</th>
                    <th className="pb-3 text-left font-medium">TinyFish</th>
                    <th className="pb-3 text-right font-medium">Diff</th>
                    <th className="pb-3 text-right font-medium">QC Time</th>
                    <th className="pb-3 text-right font-medium">TF Time</th>
                    <th className="pb-3 text-center font-medium">View</th>
                  </tr>
                </thead>
                <tbody>
                  {results.map((result, i) => {
                    const diff = result.quickcrawl.charCount - result.tinyfish.charCount
                    const diffPct = result.tinyfish.charCount > 0
                      ? ((diff / result.tinyfish.charCount) * 100).toFixed(1)
                      : "N/A"
                    return (
                      <tr key={i} className="border-b border-border">
                        <td className="py-3 pr-4">
                          <div className="font-medium">{result.publisher}</div>
                          <div className="truncate text-xs text-muted-foreground max-w-[200px]">
                            {result.url.split("/").pop()?.slice(0, 30)}...
                          </div>
                        </td>
                        <td className="py-3">
                          {result.quickcrawl.success ? (
                            <div className="flex items-center gap-2">
                              <CheckCircle className="h-4 w-4 text-green-500" />
                              <span className="font-medium">{result.quickcrawl.charCount.toLocaleString()}</span>
                            </div>
                          ) : (
                            <div className="flex items-center gap-2">
                              <XCircle className="h-4 w-4 text-red-500" />
                              <span className="text-xs text-red-500">{result.quickcrawl.error?.slice(0, 30)}</span>
                            </div>
                          )}
                        </td>
                        <td className="py-3">
                          {result.tinyfish.success ? (
                            <div className="flex items-center gap-2">
                              <CheckCircle className="h-4 w-4 text-blue-500" />
                              <span className="font-medium">{result.tinyfish.charCount.toLocaleString()}</span>
                            </div>
                          ) : (
                            <div className="flex items-center gap-2">
                              <XCircle className="h-4 w-4 text-red-500" />
                              <span className="text-xs text-red-500">{result.tinyfish.error?.slice(0, 30)}</span>
                            </div>
                          )}
                        </td>
                        <td className="py-3 text-right">
                          {result.quickcrawl.success && result.tinyfish.success ? (
                            <span className={diff >= 0 ? "text-green-500" : "text-red-500"}>
                              {diff >= 0 ? "+" : ""}{diffPct}%
                            </span>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </td>
                        <td className="py-3 text-right">
                          <span className="text-muted-foreground">{result.quickcrawl.timeMs}ms</span>
                        </td>
                        <td className="py-3 text-right">
                          <span className="text-muted-foreground">{result.tinyfish.timeMs}ms</span>
                        </td>
                        <td className="py-3 text-center">
                          <Button
                            variant="noShadow"
                            size="sm"
                            className="h-8 w-8 p-0"
                            onClick={() => setSelectedResult(result)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Summary by Publisher</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
                  {["Daily Mail", "Hindustan Times", "SCMP", "The Guardian", "NY Times"].map((pub) => {
                    const pubResults = results.filter((r) => r.publisher === pub)
                    const qcTotal = pubResults.reduce((sum, r) => sum + r.quickcrawl.charCount, 0)
                    const tfTotal = pubResults.reduce((sum, r) => sum + r.tinyfish.charCount, 0)
                    const qcAvg = pubResults.length > 0 ? qcTotal / pubResults.length : 0
                    const tfAvg = pubResults.length > 0 ? tfTotal / pubResults.length : 0
                    return (
                      <div key={pub} className="rounded-lg border border-border p-3">
                        <div className="text-sm font-medium">{pub}</div>
                        <div className="mt-2 space-y-1 text-xs">
                          <div className="flex justify-between">
                            <span className="text-green-500">QC avg:</span>
                            <span>{qcAvg.toLocaleString()} chars</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-blue-500">TF avg:</span>
                            <span>{tfAvg.toLocaleString()} chars</span>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </div>

      <TooltipProvider>
        <Sheet open={!!selectedResult} onOpenChange={() => setSelectedResult(null)}>
          <SheetContent side="right" className="w-[60%] flex flex-col">
            <SheetHeader className="flex flex-row items-center justify-between pr-8">
              <div className="flex flex-col gap-1">
                <SheetTitle className="text-lg font-bold">
                  {selectedResult?.publisher} - Content Comparison
                </SheetTitle>
                {selectedResult && (
                  <a
                    href={selectedResult.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1 text-sm text-blue-500 hover:underline"
                  >
                    {selectedResult.url}
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>
            </SheetHeader>
            {selectedResult && (
              <div className="flex-1 overflow-hidden flex flex-col mt-4">
                <Tabs defaultValue="quickcrawl" className="flex-1 overflow-hidden flex flex-col">
                  <TabsList className="mb-3">
                    <TabsTrigger value="quickcrawl" className="flex items-center gap-2">
                      <div className="h-3 w-3 rounded-full bg-green-500" />
                      QuickCrawl ({selectedResult.quickcrawl.charCount.toLocaleString()} chars)
                    </TabsTrigger>
                    <TabsTrigger value="side-by-side">Side by Side</TabsTrigger>
                    <TabsTrigger value="diff">Diff View</TabsTrigger>
                    <TabsTrigger value="tinyfish" className="flex items-center gap-2">
                      <div className="h-3 w-3 rounded-full bg-blue-500" />
                      TinyFish ({selectedResult.tinyfish.charCount.toLocaleString()} chars)
                    </TabsTrigger>
                  </TabsList>
                  <div className="flex-1 overflow-auto">
<TabsContent value="side-by-side" className="h-full">
                    <div className="grid grid-cols-2 gap-6 h-full min-h-[500px]">
                      <div className="rounded-lg border-2 border-green-500/30 bg-secondary-background p-5 overflow-auto flex flex-col">
                        <div className="flex items-center gap-2 mb-4 pb-3 border-b border-border sticky top-0 bg-secondary-background z-10">
                          <div className="h-3 w-3 rounded-full bg-green-500" />
                          <span className="font-semibold text-green-500">QuickCrawl</span>
                          <span className="text-xs text-muted-foreground ml-auto">
                            {selectedResult.quickcrawl.charCount.toLocaleString()} chars
                          </span>
                          {selectedResult.quickcrawl.success && (
                            <CheckCircle className="h-4 w-4 text-green-500 ml-2" />
                          )}
                        </div>
                        {selectedResult.quickcrawl.markdown ? (
                          <div className="prose prose-sm dark:prose-invert max-w-none flex-1">
                            <ReactMarkdown>{selectedResult.quickcrawl.markdown}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-muted-foreground">No content available</p>
                        )}
                      </div>
                      <div className="rounded-lg border-2 border-blue-500/30 bg-secondary-background p-5 overflow-auto flex flex-col">
                        <div className="flex items-center gap-2 mb-4 pb-3 border-b border-border sticky top-0 bg-secondary-background z-10">
                          <div className="h-3 w-3 rounded-full bg-blue-500" />
                          <span className="font-semibold text-blue-500">TinyFish</span>
                          <span className="text-xs text-muted-foreground ml-auto">
                            {selectedResult.tinyfish.charCount.toLocaleString()} chars
                          </span>
                          {selectedResult.tinyfish.success && (
                            <CheckCircle className="h-4 w-4 text-blue-500 ml-2" />
                          )}
                        </div>
                        {selectedResult.tinyfish.markdown ? (
                          <div className="prose prose-sm dark:prose-invert max-w-none flex-1">
                            <ReactMarkdown>{selectedResult.tinyfish.markdown}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-muted-foreground">No content available</p>
                        )}
                      </div>
                    </div>
                  </TabsContent>
                    <TabsContent value="diff" className="h-full">
                      {(() => {
                        const qc = selectedResult.quickcrawl.markdown || ""
                        const tf = selectedResult.tinyfish.markdown || ""
                        const diff = computeSimpleDiff(qc, tf)
                        const missingInQC = diff.b.filter(w => w.startsWith("[[")).length
                        const missingInTF = diff.a.filter(w => w.startsWith("[[")).length
                        return (
                          <div className="h-full overflow-auto">
                            <div className="mb-4 flex items-center gap-4 text-sm">
                              <div className="flex items-center gap-2">
                                <div className="h-3 w-3 rounded-full bg-green-500" />
                                <span className="text-green-500">QuickCrawl missing: {missingInQC} words</span>
                              </div>
                              <div className="flex items-center gap-2">
                                <div className="h-3 w-3 rounded-full bg-blue-500" />
                                <span className="text-blue-500">TinyFish missing: {missingInTF} words</span>
                              </div>
                            </div>
                            <div className="grid grid-cols-2 gap-4">
                              <div className="rounded-lg border border-green-500/30 bg-secondary-background p-4">
                                <div className="mb-2 flex items-center gap-2 border-b border-border pb-2">
                                  <div className="h-3 w-3 rounded-full bg-green-500" />
                                  <span className="font-medium text-green-500">QuickCrawl</span>
                                </div>
                                <pre className="text-xs whitespace-pre-wrap break-words font-mono max-h-[600px] overflow-auto">
                                  {diff.a.join(" ")}
                                </pre>
                              </div>
                              <div className="rounded-lg border border-blue-500/30 bg-secondary-background p-4">
                                <div className="mb-2 flex items-center gap-2 border-b border-border pb-2">
                                  <div className="h-3 w-3 rounded-full bg-blue-500" />
                                  <span className="font-medium text-blue-500">TinyFish</span>
                                </div>
                                <pre className="text-xs whitespace-pre-wrap break-words font-mono max-h-[600px] overflow-auto">
                                  {diff.b.join(" ")}
                                </pre>
                              </div>
                            </div>
                            <div className="mt-4 text-xs text-muted-foreground">
                              <span className="text-red-400">[[word]]</span> = word present in other but missing here
                            </div>
                          </div>
                        )
                      })()}
                    </TabsContent>
                    <TabsContent value="quickcrawl" className="h-full">
                      <div className="rounded-lg border border-border bg-secondary-background p-4 h-full overflow-auto">
                        {selectedResult.quickcrawl.markdown ? (
                          <div className="prose prose-sm dark:prose-invert max-w-none">
                            <ReactMarkdown>{selectedResult.quickcrawl.markdown}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-muted-foreground">No content available</p>
                        )}
                      </div>
                    </TabsContent>
                    <TabsContent value="tinyfish" className="h-full">
                      <div className="rounded-lg border border-border bg-secondary-background p-4 h-full overflow-auto">
                        {selectedResult.tinyfish.markdown ? (
                          <div className="prose prose-sm dark:prose-invert max-w-none">
                            <ReactMarkdown>{selectedResult.tinyfish.markdown}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-muted-foreground">No content available</p>
                        )}
                      </div>
                    </TabsContent>
                  </div>
                </Tabs>
              </div>
            )}
          </SheetContent>
        </Sheet>
      </TooltipProvider>
    </div>
  )
}
