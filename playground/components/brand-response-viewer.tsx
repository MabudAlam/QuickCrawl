"use client"

import { useState, useEffect } from "react"
import { ExternalLink, Copy, Check, Palette, Type, Square, Layers, Globe, Component, Ruler } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useTheme } from "next-themes"
import type { BrandResponse, BrandColor, BrandFont, BrandFonts, BrandFontLink } from "@/lib/api-types"

interface BrandResponseViewerProps {
  data: BrandResponse
  rawResponse?: unknown
  timeTakenMs?: number | null
}

export function BrandResponseViewer({ data, rawResponse, timeTakenMs }: BrandResponseViewerProps) {
  const [activeTab, setActiveTab] = useState("preview")
  const [copied, setCopied] = useState(false)
  const { resolvedTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const isDark = mounted && resolvedTheme === "dark"

  return (
    <Card className="flex h-full flex-col overflow-hidden">
      <CardHeader className="border-b-2 border-border bg-secondary-background px-4 py-3 shrink-0">
        <div className="flex items-center justify-between flex-wrap gap-2">
          <div className="flex items-center gap-3">
            <Badge variant="default" className="bg-green-600 hover:bg-green-700">
              BRAND
            </Badge>
            {data.domain && (
              <span className="text-sm font-medium">{data.domain}</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            {timeTakenMs && (
              <span className="text-xs text-muted-foreground bg-background px-2 py-1 rounded-base border border-border">
                {timeTakenMs}ms
              </span>
            )}
            <Button
              variant="neutral"
              size="sm"
              onClick={copyToClipboard}
              className="h-8 text-xs"
            >
              {copied ? <Check className="h-4 w-4 mr-1" /> : <Copy className="h-4 w-4 mr-1" />}
              {copied ? "Copied!" : "Copy"}
            </Button>
          </div>
        </div>
      </CardHeader>

      <div className="border-b border-border bg-muted/30 px-4 py-2 shrink-0">
        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList className="h-auto bg-transparent p-0 gap-1">
            <TabsTrigger
              value="preview"
              className="rounded-base data-[state=active]:bg-background data-[state=active]:shadow-sm px-4 py-1.5 text-xs font-medium"
            >
              Preview
            </TabsTrigger>
            <TabsTrigger
              value="json"
              className="rounded-base data-[state=active]:bg-background data-[state=active]:shadow-sm px-4 py-1.5 text-xs font-medium"
            >
              JSON
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <CardContent className="flex-1 overflow-auto p-0">
        {activeTab === "preview" && (
          <PreviewTab data={data} isDark={isDark} />
        )}
        {activeTab === "json" && (
          <div className="p-4 h-full">
            <pre className="h-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-4 font-mono text-xs leading-relaxed">
              {JSON.stringify(rawResponse || data, null, 2)}
            </pre>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function PreviewTab({ data, isDark }: { data: BrandResponse; isDark: boolean }) {
  const brand = data.brand
  const primaryColor = brand?.colors?.[0]?.hex || "#f54900"
  const hero = brand?.backdrops?.[0]
  const logo = brand?.logos?.[0]
  const contentRating = brand?.is_nsfw
    ? { label: "Not Safe for Work", tone: "nsfw" as const }
    : { label: "Safe for Work", tone: "sfw" as const }

  const linkEntries: { label: string; href?: string }[] = [
    { label: "Terms", href: brand?.links?.terms },
    { label: "Privacy", href: brand?.links?.privacy },
    { label: "Blog", href: brand?.links?.blog },
    { label: "Pricing", href: brand?.links?.pricing },
    { label: "Careers", href: brand?.links?.careers },
    { label: "Contact", href: brand?.links?.contact },
    { label: "Login", href: brand?.links?.login },
    { label: "Sign Up", href: brand?.links?.signup },
  ].filter((l) => l.href)

  return (
    <div className="min-h-full bg-background font-sans">
      {/* ── Sticky brand bar ── */}
      <div className="sticky top-0 z-50 flex h-11 items-center gap-3 border-b border-border bg-background/95 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div
          className="flex h-6 w-6 items-center justify-center rounded"
          style={{ backgroundColor: primaryColor }}
        >
          {logo ? (
            <img src={logo.url} alt="" className="h-4 w-4 object-contain" />
          ) : (
            <span className="text-[10px] font-bold text-white">
              {(brand?.title || brand?.name || brand?.domain || "B")[0].toUpperCase()}
            </span>
          )}
        </div>
        <span className="truncate text-sm font-semibold">
          {brand?.title || brand?.name || brand?.domain}
        </span>
        {brand?.domain && (
          <span className="ml-auto truncate text-xs text-muted-foreground">
            {brand.domain}
          </span>
        )}
        <span
          className={`ml-2 shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
            contentRating.tone === "sfw"
              ? "bg-green-500/15 text-green-600 dark:text-green-400"
              : "bg-red-500/15 text-red-600 dark:text-red-400"
          }`}
        >
          {contentRating.tone.toUpperCase()}
        </span>
      </div>

      {/* ── Hero ── */}
      <div className="relative" style={{ minHeight: "10rem" }}>
        {/* Backdrop image — z-0 so it sits behind everything */}
        <div className="relative h-40 w-full overflow-hidden bg-muted">
          {hero ? (
            <img
              src={hero.url}
              alt="Backdrop"
              className="h-full w-full object-cover"
            />
          ) : (
            <div
              className="h-full w-full"
              style={{
                background: `linear-gradient(135deg, ${primaryColor}cc, ${primaryColor}55)`,
              }}
            />
          )}
          {/* Gradient overlay — z-[5] sits above backdrop but below logo */}
          <div
            className="absolute inset-0 z-[5]"
            style={{
              background: isDark
                ? "linear-gradient(to top, rgba(0,0,0,0.7) 0%, rgba(0,0,0,0.2) 60%, transparent 100%)"
                : "linear-gradient(to top, rgba(0,0,0,0.5) 0%, rgba(0,0,0,0.1) 60%, transparent 100%)",
            }}
          />
        </div>

        {/* Logo — z-10 floats above backdrop AND gradient, overlapping into content below */}
        {logo && (
          <div
            className="absolute left-5 z-10 -mt-10 flex h-20 w-20 items-center justify-center rounded-2xl border-2 border-background bg-background shadow-xl"
            style={{ boxShadow: "0 4px 24px rgba(0,0,0,0.18)" }}
          >
            <img
              src={logo.url}
              alt="Logo"
              className="h-14 w-14 object-contain"
            />
          </div>
        )}

        {/* Hero text — z-10 above gradient but below logo */}
        <div className="relative z-10 px-5 pb-4 pt-1">
          <div className="ml-25 flex min-h-[4.5rem] flex-col justify-end">
            {brand?.title || brand?.name ? (
              <h1 className="text-2xl font-bold leading-tight text-white drop-shadow-md">
                {brand.title || brand.name}
              </h1>
            ) : null}
            {brand?.tagline && (
              <p className="mt-1 text-sm font-medium text-white/80 drop-shadow-md line-clamp-1">
                {brand.tagline}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* ── Body ── */}
      <div className="space-y-4 px-4 pb-6 pt-2">

        {/* Colors */}
        {brand?.colors && brand.colors.length > 0 && (
          <BrandColors colors={brand.colors} />
        )}

        {/* Logos + Backdrops side by side on large screens */}
        {(brand?.logos?.length ?? 0) > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {brand?.logos && brand.logos.length > 0 && (
              <div className="rounded-xl border border-border bg-card overflow-hidden">
                <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
                  <Square className="h-4 w-4 text-muted-foreground" />
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Logos
                  </h3>
                </div>
                <div className="flex flex-wrap gap-3 p-4">
                  {brand.logos.map((l, i) => (
                    <div
                      key={i}
                      className="flex h-16 w-16 items-center justify-center rounded-lg border border-border bg-secondary-background/30 p-2 transition-transform hover:scale-105"
                    >
                      <img
                        src={l.url}
                        alt={`Logo ${i + 1}`}
                        className="h-full w-full object-contain"
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}
            {brand?.backdrops && brand.backdrops.length > 0 && (
              <div className="rounded-xl border border-border bg-card overflow-hidden">
                <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
                  <Layers className="h-4 w-4 text-muted-foreground" />
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Backdrops
                  </h3>
                </div>
                <div className="flex gap-3 overflow-x-auto p-4 scrollbar-thin">
                  {brand.backdrops.map((b, i) => (
                    <a
                      key={i}
                      href={b.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="group shrink-0"
                    >
                      <div className="relative h-20 w-32 overflow-hidden rounded-lg border border-border">
                        <img
                          src={b.url}
                          alt={`Backdrop ${i + 1}`}
                          className="h-full w-full object-cover transition-transform group-hover:scale-110"
                        />
                        <div className="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover:bg-black/30">
                          <ExternalLink className="h-4 w-4 text-white opacity-0 transition-opacity group-hover:opacity-100" />
                        </div>
                      </div>
                    </a>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Industries */}
        {brand?.industries?.eic && brand.industries.eic.length > 0 && (
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
              <Globe className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Industries
              </h3>
            </div>
            <div className="flex flex-wrap gap-2 p-4">
              {brand.industries.eic.map((ind, i) => (
                <Badge key={i} variant="neutral" className="text-xs">
                  {ind.industry}
                </Badge>
              ))}
            </div>
          </div>
        )}

        {/* Description */}
        {(brand?.description || brand?.tagline) && (
          <div className="rounded-xl border border-border bg-card p-4">
            <p className="text-sm font-medium italic text-muted-foreground">
              {brand?.tagline}
            </p>
            {brand?.description && (
              <p className="mt-2 text-sm leading-relaxed text-foreground/80">
                {brand.description}
              </p>
            )}
          </div>
        )}

        {/* Fonts */}
        {brand?.fonts && brand.fonts.fonts.length > 0 && (
          <BrandFontsSection fonts={brand.fonts} />
        )}

        {/* Styleguide */}
        {brand?.styleguide && (
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
              <Component className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Styleguide
              </h3>
              <span className="ml-auto flex items-center gap-1.5">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    brand.styleguide.mode === "dark"
                      ? "bg-indigo-500"
                      : "bg-amber-400"
                  }`}
                />
                <span className="text-[10px] text-muted-foreground">
                  {brand.styleguide.mode} mode
                </span>
              </span>
            </div>
            <div className="p-4 space-y-4">
              {/* Typography */}
              {brand.styleguide.typography && (
                <div className="rounded-lg border border-border bg-secondary-background/20 p-3">
                  <p className="mb-3 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    Typography
                  </p>
                  <div className="space-y-2.5">
                    {(["h1", "h2", "h3", "h4"] as const).map((tag) => {
                      const t = brand.styleguide!.typography.headings[tag]
                      if (!t) return null
                      return (
                        <div
                          key={tag}
                          className="flex items-baseline justify-between gap-3 border-b border-border/50 pb-2 last:border-0 last:pb-0"
                        >
                          <span
                            className="truncate text-foreground"
                            style={{
                              fontFamily: t.fontFamily || "inherit",
                              fontSize: t.fontSize,
                              fontWeight: t.fontWeight,
                              lineHeight: t.lineHeight,
                              letterSpacing: t.letterSpacing,
                            }}
                          >
                            {tag.toUpperCase()} · {t.fontFamily}
                          </span>
                          <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                            {t.fontSize}/{t.fontWeight}
                          </span>
                        </div>
                      )
                    })}
                    {brand.styleguide.typography.p && (
                      <div
                        className="flex items-baseline justify-between gap-3"
                        style={{
                          fontFamily: brand.styleguide.typography.p.fontFamily || "inherit",
                          fontSize: brand.styleguide.typography.p.fontSize,
                          fontWeight: brand.styleguide.typography.p.fontWeight,
                        }}
                      >
                        <span className="truncate text-foreground">
                          Body · {brand.styleguide.typography.p.fontFamily}
                        </span>
                        <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                          {brand.styleguide.typography.p.fontSize}/{brand.styleguide.typography.p.fontWeight}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              )}

              {/* Components */}
              <div className="rounded-lg border border-border bg-secondary-background/20 p-3">
                <p className="mb-3 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  Components
                </p>
                <div className="flex flex-wrap items-center gap-3">
                  {brand.styleguide.components?.button?.primary?.css && (
                    <button
                      style={{
                        backgroundColor: brand.styleguide.components.button.primary.backgroundColor || undefined,
                        color: brand.styleguide.components.button.primary.color || undefined,
                        borderColor: brand.styleguide.components.button.primary.borderColor || undefined,
                        borderRadius: brand.styleguide.components.button.primary.borderRadius || undefined,
                        borderWidth: brand.styleguide.components.button.primary.borderWidth || undefined,
                        borderStyle: brand.styleguide.components.button.primary.borderStyle || undefined,
                        padding: brand.styleguide.components.button.primary.padding || undefined,
                        fontSize: brand.styleguide.components.button.primary.fontSize || undefined,
                        fontWeight: brand.styleguide.components.button.primary.fontWeight || undefined,
                        boxShadow: brand.styleguide.components.button.primary.boxShadow || undefined,
                        fontFamily: brand.styleguide.components.button.primary.fontFamily || undefined,
                      }}
                    >
                      Primary
                    </button>
                  )}
                  {brand.styleguide.components?.button?.secondary?.css && (
                    <button
                      style={{
                        backgroundColor: brand.styleguide.components.button.secondary.backgroundColor || undefined,
                        color: brand.styleguide.components.button.secondary.color || undefined,
                        borderColor: brand.styleguide.components.button.secondary.borderColor || undefined,
                        borderRadius: brand.styleguide.components.button.secondary.borderRadius || undefined,
                        borderWidth: brand.styleguide.components.button.secondary.borderWidth || undefined,
                        borderStyle: brand.styleguide.components.button.secondary.borderStyle || undefined,
                        padding: brand.styleguide.components.button.secondary.padding || undefined,
                        fontSize: brand.styleguide.components.button.secondary.fontSize || undefined,
                        fontWeight: brand.styleguide.components.button.secondary.fontWeight || undefined,
                        boxShadow: brand.styleguide.components.button.secondary.boxShadow || undefined,
                        fontFamily: brand.styleguide.components.button.secondary.fontFamily || undefined,
                      }}
                    >
                      Secondary
                    </button>
                  )}
                  {brand.styleguide.components?.button?.link?.css && (
                    <a
                      href="#"
                      onClick={(e) => e.preventDefault()}
                      style={{
                        color: brand.styleguide.components.button.link.color || undefined,
                        textDecoration: brand.styleguide.components.button.link.textDecoration || undefined,
                        fontFamily: brand.styleguide.components.button.link.fontFamily || undefined,
                        fontSize: brand.styleguide.components.button.link.fontSize || undefined,
                      }}
                    >
                      Link
                    </a>
                  )}
                  {brand.styleguide.components?.card?.css && (
                    <div
                      className="min-w-[120px] text-xs"
                      style={{
                        backgroundColor: brand.styleguide.components.card.backgroundColor || undefined,
                        color: brand.styleguide.components.card.textColor || undefined,
                        borderColor: brand.styleguide.components.card.borderColor || undefined,
                        borderRadius: brand.styleguide.components.card.borderRadius || undefined,
                        borderWidth: brand.styleguide.components.card.borderWidth || undefined,
                        borderStyle: brand.styleguide.components.card.borderStyle || undefined,
                        padding: brand.styleguide.components.card.padding || undefined,
                        boxShadow: brand.styleguide.components.card.boxShadow || undefined,
                      }}
                    >
                      Card
                    </div>
                  )}
                </div>
              </div>

              {/* Spacing + Shadows */}
              <div className="grid grid-cols-2 gap-2">
                {brand.styleguide.elementSpacing && (
                  <div className="rounded-lg border border-border bg-secondary-background/20 p-3">
                    <p className="mb-2 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                      <Ruler className="h-3 w-3" /> Spacing
                    </p>
                    <div className="space-y-1 text-xs">
                      {Object.entries(brand.styleguide.elementSpacing).map(([k, v]) => (
                        <div key={k} className="flex justify-between">
                          <span className="font-mono">{k}</span>
                          <span className="text-muted-foreground">{v}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                {brand.styleguide.shadows && (
                  <div className="rounded-lg border border-border bg-secondary-background/20 p-3">
                    <p className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                      Shadows
                    </p>
                    <div className="space-y-2">
                      {Object.entries(brand.styleguide.shadows).map(([k, v]) => (
                        <div key={k} className="text-xs">
                          <span className="font-mono">{k}</span>
                          <div
                            className="mt-1 h-5 w-full rounded bg-background"
                            style={{ boxShadow: v === "none" ? undefined : v }}
                          />
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Contact + Links */}
        {(brand?.email || (brand?.socials?.length ?? 0) > 0 || linkEntries.length > 0) && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Contact */}
            {(brand?.email || (brand?.socials?.length ?? 0) > 0) && (
              <div className="rounded-xl border border-border bg-card overflow-hidden">
                <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
                  <Globe className="h-4 w-4 text-muted-foreground" />
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Contact
                  </h3>
                </div>
                <div className="divide-y divide-border">
                  {brand?.email && (
                    <a
                      href={`mailto:${brand.email}`}
                      className="flex items-center gap-3 px-4 py-3 text-sm transition-colors hover:bg-secondary-background/50"
                    >
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-secondary-background">
                        <span className="text-sm">@</span>
                      </div>
                      <div className="min-w-0">
                        <p className="text-[10px] text-muted-foreground">Email</p>
                        <p className="truncate font-medium">{brand.email}</p>
                      </div>
                    </a>
                  )}
                  {brand?.socials?.map((social, i) => (
                    <a
                      key={i}
                      href={social.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center justify-between px-4 py-3 text-sm transition-colors hover:bg-secondary-background/50"
                    >
                      <span className="capitalize">{social.type}</span>
                      <ExternalLink className="h-3.5 w-3.5 text-muted-foreground" />
                    </a>
                  ))}
                </div>
              </div>
            )}

            {/* Links */}
            {linkEntries.length > 0 && (
              <div className="rounded-xl border border-border bg-card overflow-hidden">
                <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
                  <ExternalLink className="h-4 w-4 text-muted-foreground" />
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Links
                  </h3>
                </div>
                <div className="grid grid-cols-2 divide-x divide-border">
                  {linkEntries.map((l) => (
                    <a
                      key={l.label}
                      href={l.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center justify-center gap-2 px-3 py-3 text-xs transition-colors hover:bg-secondary-background/50"
                    >
                      <span className="font-medium">{l.label}</span>
                      <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
                    </a>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Address */}
        {brand?.address &&
          (brand.address.city || brand.address.country || brand.address.postal_code) && (
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
                <Globe className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  Address
                </h3>
              </div>
              <div className="p-4 text-sm">
                {brand.address.city && (
                  <p>
                    {brand.address.city}
                    {brand.address.state_province ? `, ${brand.address.state_province}` : ""}{" "}
                    {brand.address.postal_code}
                  </p>
                )}
                {brand.address.country && <p className="text-muted-foreground">{brand.address.country}</p>}
              </div>
            </div>
          )}
      </div>
    </div>
  )
}

function BrandColors({ colors }: { colors: BrandColor[] | undefined }) {
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null)

  if (!colors || colors.length === 0) return null

  const copy = (hex: string, index: number) => {
    navigator.clipboard.writeText(hex)
    setCopiedIndex(index)
    setTimeout(() => setCopiedIndex(null), 1500)
  }

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
        <Palette className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Colors
        </h3>
      </div>
      <div className="p-4">
        <div className="flex flex-wrap gap-3">
          {colors.map((c: BrandColor, i: number) => (
            <button
              key={i}
              onClick={() => copy(c.hex, i)}
              className="group flex items-center gap-2.5 rounded-lg border border-border bg-secondary-background/30 px-3 py-2 transition-all hover:border-foreground/20 hover:bg-secondary-background/50 active:scale-95"
              title={`Copy ${c.hex}`}
            >
              <div
                className="h-7 w-7 shrink-0 rounded-full border border-border shadow-sm"
                style={{ backgroundColor: c.hex }}
              />
              <div className="text-left">
                <p className="text-xs font-medium capitalize">{c.name}</p>
                <p className="font-mono text-[10px] text-muted-foreground">
                  {copiedIndex === i ? (
                    <span className="flex items-center gap-0.5 text-green-600 dark:text-green-400">
                      <Check className="h-3 w-3" /> copied
                    </span>
                  ) : (
                    c.hex
                  )}
                </p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}

function BrandFontsSection({ fonts }: { fonts: BrandFonts | undefined }) {
  const [loadedFamilies, setLoadedFamilies] = useState<Set<string>>(new Set())
  const [mounted, setMounted] = useState(false)
  const [activeFamily, setActiveFamily] = useState<string | null>(null)

  useEffect(() => {
    setMounted(true)
    if (!fonts) return
    const families = fonts.fonts.map((f: BrandFont) => f.font).filter(Boolean)
    if (families.length === 0) return
    setActiveFamily(families[0])

    const fontLinks = fonts.fontLinks || {}
    const allURLs: { family: string; url: string }[] = []
    for (const [family, fl] of Object.entries(fontLinks) as [string, BrandFontLink][]) {
      if (!fl?.files) continue
      for (const [, url] of Object.entries(fl.files)) {
        allURLs.push({ family, url })
      }
    }

    const loadFonts = async () => {
      for (const { family, url } of allURLs) {
        try {
          const fontFace = new FontFace(family, `url(${url})`)
          await fontFace.load()
          document.fonts.add(fontFace)
          setLoadedFamilies((prev) => new Set([...prev, family]))
        } catch {
        }
      }
    }

    loadFonts()
  }, [fonts])

  if (!fonts || fonts.fonts.length === 0) return null

  const families = fonts.fonts.map((f: BrandFont) => f.font).filter(Boolean)

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
        <Type className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Fonts
        </h3>
        {mounted && loadedFamilies.size > 0 && (
          <span className="ml-2 rounded bg-green-500/15 px-1.5 py-0.5 text-[10px] font-medium text-green-600 dark:text-green-400">
            {loadedFamilies.size} loaded
          </span>
        )}
      </div>

      <div className="p-4 space-y-4">
        {/* Font tabs */}
        <div className="flex flex-wrap gap-1.5">
          {families.map((f) => (
            <button
              key={f}
              onClick={() => setActiveFamily(f)}
              className={`rounded-full border px-3 py-1 text-xs font-medium transition-all ${
                activeFamily === f
                  ? "border-foreground/30 bg-foreground text-background"
                  : "border-border bg-secondary-background/30 hover:bg-secondary-background/60"
              }`}
            >
              {f}
            </button>
          ))}
        </div>

        {/* Live preview */}
        {activeFamily && (
          <div
            className="rounded-lg border border-border bg-background p-4"
            style={{
              fontFamily: mounted && loadedFamilies.has(activeFamily) ? activeFamily : "inherit",
            }}
          >
            <p className="text-3xl font-bold leading-tight">
              The quick brown fox jumps over the lazy dog
            </p>
            <p className="mt-2 text-lg">
                              ABCDEFGHIJKLMNOPQRSTUVWXYZ
            </p>
            <p className="text-sm uppercase tracking-widest text-muted-foreground">
                              abcdefghijklmnopqrstuvwxyz 0123456789
            </p>
          </div>
        )}

        {/* Font list */}
        <div className="space-y-2">
          {fonts.fonts.map((f: BrandFont, i: number) => (
            <div
              key={i}
              className="flex items-center justify-between rounded-lg border border-border bg-secondary-background/20 px-3 py-2"
            >
              <div className="flex items-center gap-3">
                <span
                  className="text-sm font-medium"
                  style={{
                    fontFamily: mounted && loadedFamilies.has(f.font) ? f.font : undefined,
                  }}
                >
                  {f.font}
                </span>
                {loadedFamilies.has(f.font) && (
                  <span className="rounded bg-green-500/15 px-1 py-0.5 text-[9px] font-semibold text-green-600 dark:text-green-400">
                    LOADED
                  </span>
                )}
              </div>
              <div className="flex items-center gap-3">
                <div className="h-1.5 w-24 overflow-hidden rounded-full bg-secondary-background">
                  <div
                    className="h-full rounded-full"
                    style={{
                      width: `${f.percent_elements}%`,
                      backgroundColor: primaryColorForFont(f.percent_elements),
                    }}
                  />
                </div>
                <span className="font-mono text-[10px] text-muted-foreground">
                  {f.percent_elements}%
                </span>
              </div>
            </div>
          ))}
        </div>

        {/* Font files */}
        {fonts.fontLinks && Object.keys(fonts.fontLinks).length > 0 && (
          <div className="rounded-lg border border-border bg-secondary-background/20 p-3">
            <p className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Font Files
            </p>
            <div className="space-y-2">
              {(Object.entries(fonts.fontLinks) as [string, BrandFontLink][]).map(([name, fl]) => (
                <div key={name} className="text-xs">
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-medium">{name}</span>
                    <Badge variant="neutral" className="text-[10px]">
                      {fl.type}
                    </Badge>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {Object.entries(fl.files).map(([weight, url]) => (
                      <a
                        key={weight}
                        href={url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] hover:bg-secondary-background/50"
                      >
                        {weight}
                      </a>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function primaryColorForFont(percent: number): string {
  if (percent >= 70) return "#16a34a"
  if (percent >= 40) return "#ca8a04"
  return "#dc2626"
}

function Section({
  title,
  icon,
  children,
}: {
  title: string
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex items-center gap-2 border-b border-border bg-secondary-background/50 px-4 py-2.5">
        {icon}
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {title}
        </h3>
      </div>
      <div className="p-4">{children}</div>
    </div>
  )
}
