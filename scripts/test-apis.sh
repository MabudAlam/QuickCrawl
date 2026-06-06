#!/bin/bash

# API Test Script for quickcrawl
# Tests scrape and crawl APIs against various websites

BASE_URL="${BASE_URL:-http://0.0.0.0:3000}"
TIMEOUT=30
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test results storage
TESTS_PASSED=0
TESTS_FAILED=0
GO_TESTS_PASSED=0
GO_TESTS_FAILED=0
MCP_TESTS_PASSED=0
MCP_TESTS_FAILED=0
CLI_TESTS_PASSED=0
CLI_TESTS_FAILED=0

# Function to test scrape API
test_scrape() {
    local url="$1"
    local test_name="$2"

    echo -e "\n${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Testing Scrape: $test_name${NC}"
    echo -e "${YELLOW}URL: $url${NC}"
    echo -e "${YELLOW}========================================${NC}"

    response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/v1/scrape" \
        -H 'Content-Type: application/json' \
        -d "{\"url\":\"$url\",\"formats\":[\"markdown\"]}" \
        --max-time $TIMEOUT)

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" != "200" ]; then
        echo -e "${RED}❌ FAIL: HTTP $http_code${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    # Check if valid JSON
    if ! echo "$body" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
        echo -e "${RED}❌ FAIL: Invalid JSON response${NC}"
        echo "Response preview: $(echo "$body" | head -c 200)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    # Check for success field
    success=$(echo "$body" | python3 -c "import sys,json; print(json.load(sys.stdin).get('success', False))" 2>/dev/null)
    if [ "$success" != "True" ]; then
        echo -e "${RED}❌ FAIL: success=false${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    # Check for markdown content
    markdown_len=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); m=d.get('data',{}).get('markdown'); print(len(m) if m else 0)" 2>/dev/null)
    if [ "$markdown_len" -gt 100 ]; then
        echo -e "${GREEN}✅ PASS: Valid markdown ($markdown_len chars)${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}❌ FAIL: Markdown too short ($markdown_len chars)${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Function to test crawl API
test_crawl() {
    local url="$1"
    local test_name="$2"
    local max_pages="${3:-1}"

    echo -e "\n${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Testing Crawl: $test_name${NC}"
    echo -e "${YELLOW}URL: $url (max_pages=$max_pages)${NC}"
    echo -e "${YELLOW}========================================${NC}"

    # Start crawl
    start_response=$(curl -s -X POST "$BASE_URL/v1/crawl" \
        -H 'Content-Type: application/json' \
        -d "{\"url\":\"$url\",\"maxDepth\":0,\"maxPages\":$max_pages,\"formats\":[\"markdown\"],\"renderJs\":false}" \
        --max-time $TIMEOUT)

    if ! echo "$start_response" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
        echo -e "${RED}❌ FAIL: Invalid JSON response when starting crawl${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    crawl_id=$(echo "$start_response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id', ''))" 2>/dev/null)
    if [ -z "$crawl_id" ]; then
        echo -e "${RED}❌ FAIL: No crawl ID returned${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
    echo "Crawl ID: $crawl_id"

    # Poll for completion (max 30 seconds)
    max_attempts=15
    attempt=0
    while [ $attempt -lt $max_attempts ]; do
        sleep 2
        status_response=$(curl -s "$BASE_URL/v1/crawl/$crawl_id" --max-time 10)

        if ! echo "$status_response" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
            echo -e "${RED}❌ FAIL: Invalid JSON in status response${NC}"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            return 1
        fi

        status=$(echo "$status_response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status', ''))" 2>/dev/null)
        echo "Status: $status (attempt $((attempt+1))/$max_attempts)"

        if [ "$status" = "completed" ]; then
            # Check for valid results
            result_count=$(echo "$status_response" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data', [])))" 2>/dev/null)
            if [ "$result_count" -gt 0 ]; then
                markdown_len=$(echo "$status_response" | python3 -c "import sys,json; d=json.load(sys.stdin); m=d.get('data', [{}])[0].get('markdown'); print(len(m) if m else 0)" 2>/dev/null)
                if [ "$markdown_len" -gt 100 ]; then
                    echo -e "${GREEN}✅ PASS: Crawl completed with $result_count result(s), markdown ($markdown_len chars)${NC}"
                    TESTS_PASSED=$((TESTS_PASSED + 1))
                    return 0
                else
                    echo -e "${RED}❌ FAIL: Markdown too short ($markdown_len chars)${NC}"
                    TESTS_FAILED=$((TESTS_FAILED + 1))
                    return 1
                fi
            else
                echo -e "${RED}❌ FAIL: No results returned${NC}"
                TESTS_FAILED=$((TESTS_FAILED + 1))
                return 1
            fi
        elif [ "$status" = "failed" ]; then
            echo -e "${RED}❌ FAIL: Crawl failed${NC}"
            error=$(echo "$status_response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('error', 'unknown'))" 2>/dev/null)
            echo "Error: $error"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            return 1
        fi

        attempt=$((attempt + 1))
    done

    echo -e "${RED}❌ FAIL: Crawl timed out${NC}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
}

# Check if server is running
echo -e "${YELLOW}Checking if server is running at $BASE_URL...${NC}"
if ! curl -s --max-time 5 "$BASE_URL/health" > /dev/null 2>&1; then
    echo -e "${RED}❌ ERROR: Server not responding at $BASE_URL${NC}"
    echo "Start the server with: go run ./cmd/server/main.go"
    exit 1
fi
echo -e "${GREEN}✅ Server is running${NC}"

# Run Go unit tests
run_go_tests() {
    echo ""
    echo "========================================"
    echo "Running Go Unit Tests"
    echo "========================================"

    cd "$PROJECT_ROOT"

    # Run all tests
    if go test -v ./... 2>&1 | tee /tmp/go-test-output.txt; then
        echo -e "${GREEN}✅ All Go tests passed${NC}"
        GO_TESTS_FAILED=0
    else
        echo -e "${RED}❌ Some Go tests failed${NC}"
        GO_TESTS_FAILED=1
    fi

    # Run MCP tests separately to get count
    echo ""
    echo "--- MCP Tests ---"
    mcp_output=$(go test -v ./internal/mcp/... 2>&1)
    mcp_passed=$(echo "$mcp_output" | grep -c "^\-\-\- PASS" || true)
    mcp_failed=$(echo "$mcp_output" | grep -c "^\-\-\- FAIL" || true)
    mcp_passed=${mcp_passed:-0}
    mcp_failed=${mcp_failed:-0}
    echo "Passed: $mcp_passed, Failed: $mcp_failed"
    MCP_TESTS_PASSED=$mcp_passed
    MCP_TESTS_FAILED=$mcp_failed

    # Run CLI tests separately to get count
    echo ""
    echo "--- CLI Tests ---"
    cli_output=$(go test -v ./cli/... 2>&1)
    cli_passed=$(echo "$cli_output" | grep -c "^\-\-\- PASS" || true)
    cli_failed=$(echo "$cli_output" | grep -c "^\-\-\- FAIL" || true)
    cli_passed=${cli_passed:-0}
    cli_failed=${cli_failed:-0}
    echo "Passed: $cli_passed, Failed: $cli_failed"
    CLI_TESTS_PASSED=$cli_passed
    CLI_TESTS_FAILED=$cli_failed

    # Total Go tests
    total=$(grep -c "^\-\-\- PASS" /tmp/go-test-output.txt 2>/dev/null || true)
    total=${total:-0}
    GO_TESTS_PASSED=$total
}

run_go_tests

echo ""
echo "========================================"
echo "Starting API Tests"
echo "========================================"

# Test 1: Substack redirect (tests redirect handling)
test_scrape "https://substack.com/home/post/p-190088982" "Substack Redirect"
test_crawl "https://substack.com/home/post/p-190088982" "Substack Redirect" 1

# Test 2: Personal site (mabud.dev)
test_scrape "https://www.mabud.dev/" "Personal Site (mabud.dev)"
test_crawl "https://www.mabud.dev/" "Personal Site (mabud.dev)" 1

# Test 3: Featsclub (tests site with more dynamic content)
test_scrape "https://www.featsclub.com/" "Featsclub"
test_crawl "https://www.featsclub.com/" "Featsclub" 1

# Test 4: Notion (tests SPA/rendering)
test_scrape "https://www.notion.com/" "Notion"
test_crawl "https://www.notion.com/" "Notion" 1

# Summary
echo ""
echo "========================================"
echo "Test Summary"
echo "========================================"
echo -e "${YELLOW}Go Unit Tests${NC}: ${GREEN}Passed: $GO_TESTS_PASSED${NC} ${RED}Failed: $GO_TESTS_FAILED${NC}"
echo -e "${YELLOW}MCP Tests${NC}: ${GREEN}Passed: $MCP_TESTS_PASSED${NC} ${RED}Failed: $MCP_TESTS_FAILED${NC}"
echo -e "${YELLOW}CLI Tests${NC}: ${GREEN}Passed: $CLI_TESTS_PASSED${NC} ${RED}Failed: $CLI_TESTS_FAILED${NC}"
echo -e "${YELLOW}API Tests${NC}: ${GREEN}Passed: $TESTS_PASSED${NC} ${RED}Failed: $TESTS_FAILED${NC}"

if [ $TESTS_FAILED -eq 0 ] && [ $GO_TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed! ✅${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed! ❌${NC}"
    exit 1
fi