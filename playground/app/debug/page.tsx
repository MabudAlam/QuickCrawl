"use client";

import { useState, useMemo, useCallback, useRef } from "react";
import { Loader2, FileJson, ArrowUpDown, ArrowUp, ArrowDown, RefreshCw, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type FlushResult = {
  url: string;
  success: boolean;
  status_code: number;
  response_time_ms: number;
  content_verified: boolean;
  noise_detected: boolean;
  error: string;
};

type FlushData = {
  errors: Record<string, number>;
  failed_urls: { url: string; reason: string; status: number }[];
  results: FlushResult[];
};

const BENCH_DIR = "/Users/skmabudalam/Desktop/gocrawl/bench";

type SortField = "response_time_ms" | "status_code" | "url" | "error";
type SortDirection = "asc" | "desc";

export default function DebugPage() {
  const [selectedFile, setSelectedFile] = useState<string>("");
  const [flushData, setFlushData] = useState<FlushData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [availableFiles, setAvailableFiles] = useState<string[]>([]);
  const [sortField, setSortField] = useState<SortField>("response_time_ms");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadFiles = async () => {
    setIsLoading(true);
    try {
      const files = await fetch("/api/debug/files").then((r) => r.json());
      setAvailableFiles(files);
      if (files.length > 0 && !selectedFile) {
        setSelectedFile(files[0]);
      }
    } catch (e) {
      console.error("Failed to load files:", e);
      setAvailableFiles([]);
    }
    setIsLoading(false);
  };

  const loadFlushData = async (file: string) => {
    if (!file) return;
    setIsLoading(true);
    try {
      const data = await fetch(`/api/debug/flush?file=${encodeURIComponent(file)}`).then((r) =>
        r.json()
      );
      setFlushData(data);
    } catch (e) {
      console.error("Failed to load flush data:", e);
      setFlushData(null);
    }
    setIsLoading(false);
  };

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    const files = Array.from(e.dataTransfer.files);
    const jsonFile = files.find((f) => f.name.endsWith(".json"));
    if (jsonFile) {
      const reader = new FileReader();
      reader.onload = (event) => {
        try {
          const data = JSON.parse(event.target?.result as string);
          setFlushData(data);
          setSelectedFile(jsonFile.name);
        } catch (err) {
          console.error("Failed to parse JSON:", err);
        }
      };
      reader.readAsText(jsonFile);
    }
  }, []);

  const handleFileInput = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (event) => {
        try {
          const data = JSON.parse(event.target?.result as string);
          setFlushData(data);
          setSelectedFile(file.name);
        } catch (err) {
          console.error("Failed to parse JSON:", err);
        }
      };
      reader.readAsText(file);
    }
  }, []);

  useMemo(() => {
    loadFiles();
  }, []);

  const handleFileChange = (file: string) => {
    setSelectedFile(file);
    loadFlushData(file);
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("desc");
    }
  };

  const sortedResults = useMemo(() => {
    if (!flushData?.results) return [];

    return [...flushData.results].sort((a, b) => {
      let aVal: string | number | boolean = "";
      let bVal: string | number | boolean = "";

      switch (sortField) {
        case "response_time_ms":
          aVal = a.response_time_ms;
          bVal = b.response_time_ms;
          break;
        case "status_code":
          aVal = a.status_code;
          bVal = b.status_code;
          break;
        case "url":
          aVal = a.url;
          bVal = b.url;
          break;
        case "error":
          aVal = a.error || (a.success ? "" : "unknown");
          bVal = b.error || (b.success ? "" : "unknown");
          break;
      }

      if (typeof aVal === "number" && typeof bVal === "number") {
        return sortDirection === "asc" ? aVal - bVal : bVal - aVal;
      }

      const aStr = String(aVal).toLowerCase();
      const bStr = String(bVal).toLowerCase();
      if (sortDirection === "asc") {
        return aStr.localeCompare(bStr);
      }
      return bStr.localeCompare(aStr);
    });
  }, [flushData, sortField, sortDirection]);

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) {
      return <ArrowUpDown className="h-4 w-4 opacity-50" />;
    }
    return sortDirection === "asc" ? (
      <ArrowUp className="h-4 w-4" />
    ) : (
      <ArrowDown className="h-4 w-4" />
    );
  };

  const stats = useMemo(() => {
    if (!flushData?.results) return null;
    const results = flushData.results;
    const total = results.length;
    const success = results.filter((r) => r.success).length;
    const failed = total - success;
    const times = results.map((r) => r.response_time_ms).filter((t) => t > 0);
    const avg = times.reduce((a, b) => a + b, 0) / times.length;
    const sorted = [...times].sort((a, b) => a - b);
    const p50 = sorted[Math.floor(sorted.length * 0.5)] || 0;
    const p95 = sorted[Math.floor(sorted.length * 0.95)] || 0;
    const p99 = sorted[Math.floor(sorted.length * 0.99)] || 0;

    return { total, success, failed, avg, p50, p95, p99 };
  }, [flushData]);

  return (
    <div className="container mx-auto p-8 max-w-[1800px]">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Benchmark Debug</h1>
          <p className="text-muted-foreground mt-1">
            View and analyze benchmark flush results
          </p>
        </div>
        <Button variant="neutral" size="sm" onClick={loadFiles} disabled={isLoading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${isLoading ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      <div className="mb-6 flex gap-4 items-start">
        <div className="flex-1">
          <Select value={selectedFile || "none"} onValueChange={handleFileChange}>
            <SelectTrigger className="w-[400px]">
              <SelectValue placeholder="Select a flush file..." />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none" disabled>Select a flush file...</SelectItem>
              {availableFiles.map((file) => (
                <SelectItem key={file} value={file}>
                  {file}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex-1">
          <div
            className={cn(
              "border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-colors",
              isDragging
                ? "border-primary bg-primary/10"
                : "border-muted-foreground/30 hover:border-muted-foreground/50",
              flushData && "border-green-500 bg-green-500/10"
            )}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".json"
              className="hidden"
              onChange={handleFileInput}
            />
            <Upload className={cn("h-8 w-8 mx-auto mb-2", isDragging ? "text-primary" : "text-muted-foreground")} />
            <p className="text-sm font-medium">
              {flushData ? selectedFile || "JSON loaded" : "Drag & drop flush.json here"}
            </p>
            <p className="text-xs text-muted-foreground mt-1">
              or click to browse
            </p>
          </div>
        </div>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      )}

      {flushData && !isLoading && (
        <>
          {stats && (
            <div className="grid grid-cols-4 gap-4 mb-8">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Total URLs
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{stats.total}</div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Success Rate
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {((stats.success / stats.total) * 100).toFixed(1)}%
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {stats.success} / {stats.total}
                  </p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Avg Response
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{Math.round(stats.avg)}ms</div>
                  <p className="text-xs text-muted-foreground">
                    P50: {Math.round(stats.p50)}ms | P99: {Math.round(stats.p99)}ms
                  </p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Failed
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-destructive">{stats.failed}</div>
                  <p className="text-xs text-muted-foreground">
                    {Object.keys(flushData.errors).length} error types
                  </p>
                </CardContent>
              </Card>
            </div>
          )}

          {Object.keys(flushData.errors).length > 0 && (
            <div className="mb-6">
              <h3 className="text-sm font-medium mb-3">Top Errors</h3>
              <div className="flex flex-wrap gap-2">
                {Object.entries(flushData.errors)
                  .sort((a, b) => b[1] - a[1])
                  .map(([error, count]) => (
                    <Badge key={error} variant="neutral">
                      {error}: {count}
                    </Badge>
                  ))}
              </div>
            </div>
          )}

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileJson className="h-5 w-5" />
                Results ({sortedResults.length.toLocaleString()} entries)
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <div className="relative overflow-auto max-h-[600px]">
                <Table>
                  <TableHeader className="sticky top-0 bg-muted/50 backdrop-blur">
                    <TableRow>
                      <TableHead
                        className="cursor-pointer hover:bg-muted"
                        onClick={() => handleSort("url")}
                      >
                        <div className="flex items-center gap-2">
                          URL <SortIcon field="url" />
                        </div>
                      </TableHead>
                      <TableHead
                        className="cursor-pointer hover:bg-muted"
                        onClick={() => handleSort("status_code")}
                      >
                        <div className="flex items-center gap-2">
                          Status <SortIcon field="status_code" />
                        </div>
                      </TableHead>
                      <TableHead
                        className="cursor-pointer hover:bg-muted"
                        onClick={() => handleSort("response_time_ms")}
                      >
                        <div className="flex items-center gap-2">
                          Response Time <SortIcon field="response_time_ms" />
                        </div>
                      </TableHead>
                      <TableHead className="text-center">Success</TableHead>
                      <TableHead className="text-center">Content Verified</TableHead>
                      <TableHead className="text-center">Noise Detected</TableHead>
                      <TableHead
                        className="cursor-pointer hover:bg-muted"
                        onClick={() => handleSort("error")}
                      >
                        <div className="flex items-center gap-2">
                          Error <SortIcon field="error" />
                        </div>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {sortedResults.map((result, idx) => (
                      <TableRow
                        key={idx}
                        className={!result.success ? "bg-destructive/5" : ""}
                      >
                        <TableCell className="max-w-[300px] truncate font-mono text-xs">
                          <a
                            href={result.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="hover:underline"
                          >
                            {result.url}
                          </a>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={
                              result.status_code >= 500
                                ? "neutral"
                                : result.status_code >= 400
                                ? "neutral"
                                : "default"
                            }
                          >
                            {result.status_code}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono">
                          <span
                            className={
                              result.response_time_ms > 10000
                                ? "text-destructive font-bold"
                                : result.response_time_ms > 5000
                                ? "text-yellow-600"
                                : ""
                            }
                          >
                            {result.response_time_ms.toFixed(0)}ms
                          </span>
                        </TableCell>
                        <TableCell className="text-center">
                          {result.success ? (
                            <span className="text-green-600">Yes</span>
                          ) : (
                            <span className="text-destructive">No</span>
                          )}
                        </TableCell>
                        <TableCell className="text-center">
                          {result.content_verified ? (
                            <span className="text-green-600">Yes</span>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </TableCell>
                        <TableCell className="text-center">
                          {result.noise_detected ? (
                            <span className="text-yellow-600">Yes</span>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-[200px] truncate">
                          {result.error || "-"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </>
      )}

      {!flushData && !isLoading && (
        <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
          <FileJson className="h-12 w-12 mb-4 opacity-50" />
          <p>Select a flush file to view results</p>
        </div>
      )}
    </div>
  );
}
