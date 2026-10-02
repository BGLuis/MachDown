#!/bin/bash
# =============================================================================
# API Test Script
# Generated for: MachDown Server Core
# =============================================================================
#
# Usage: ./scripts/test-api.sh [API_KEY] [BASE_URL]
#        Default BASE_URL: https://localhost:8888
#
# Requirements:
# - No external dependencies (no jq, only curl and bash)
# - Each request has -m 10 timeout to prevent hanging
# - HTTP status captured via: -o /tmp/response.txt -w "%{http_code}"
#
# =============================================================================
#
# TEST CASE OVERVIEW (Human-Reviewable)
# =============================================================================
#
# ┌─────────────────────────────────────────────────────────────────────────────┐
# │ VALIDATION ERROR TESTS                                                      │
# ├─────────┬──────────────────────┬────────────┬────────────────────────┬──────┤
# │ Test ID │ Description          │ API Key    │ Payload                │ HTTP │
# ├─────────┼──────────────────────┼────────────┼────────────────────────┼──────┤
# │ AC1.1   │ Missing API Key      │ (none)     │ {"url": "http..."}     │ 401  │
# │ AC1.2   │ Invalid API Key      │ "wrong-key"│ {"url": "http..."}     │ 401  │
# │ AC1.3   │ Missing URL          │ Valid Key  │ {}                     │ 400  │
# └─────────┴──────────────────────┴────────────┴────────────────────────┴──────┘
#
# ┌─────────────────────────────────────────────────────────────────────────────┐
# │ SUCCESS TESTS                                                               │
# ├─────────┬──────────────────────┬────────────┬────────────────────────┬──────┤
# │ Test ID │ Description          │ API Key    │ Payload                │ HTTP │
# ├─────────┼──────────────────────┼────────────┼────────────────────────┼──────┤
# │ AC2.1   │ Enqueue Download     │ Valid Key  │ {"url": "http...",...} │ 201  │
# └─────────┴──────────────────────┴────────────┴────────────────────────┴──────┘
#
# =============================================================================

# -----------------------------------------------------------------------------
# CONFIGURATION
# -----------------------------------------------------------------------------
API_KEY="${1:-}"
BASE_URL="${2:-https://localhost:8888}"

if [ -z "$API_KEY" ]; then
    echo "Error: API_KEY is required as the first argument."
    echo "Usage: ./scripts/test-api.sh <YOUR_API_KEY> [BASE_URL]"
    exit 1
fi

# Colors for output
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    CYAN=''
    NC=''
fi

# -----------------------------------------------------------------------------
# TEST COUNTERS AND RESULT TRACKING
# -----------------------------------------------------------------------------
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

declare -a TEST_IDS
declare -a TEST_DESCRIPTIONS
declare -a EXPECTED_STATUS
declare -a ACTUAL_STATUS
declare -a TEST_RESULTS

# -----------------------------------------------------------------------------
# HELPER FUNCTIONS
# -----------------------------------------------------------------------------
print_test_header() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}TEST: $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
}

print_expected() {
    echo -e "${YELLOW}Expected: $1${NC}"
}

print_result() {
    echo -e "${GREEN}Response:${NC}"
}

record_result() {
    TEST_IDS+=("$1")
    TEST_DESCRIPTIONS+=("$2")
    EXPECTED_STATUS+=("$3")
    ACTUAL_STATUS+=("$4")
    TEST_RESULTS+=("$5")
}

check_result() {
    local test_id="$1"
    local test_desc="$2"
    local expected_status="$3"
    local actual_status="$4"
    local body="$5"

    echo "$body"
    echo ""

    if [ "$actual_status" = "$expected_status" ]; then
        echo -e "${GREEN}✓ PASSED${NC} [HTTP Status: $actual_status]"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        record_result "$test_id" "$test_desc" "$expected_status" "$actual_status" "PASS"
    else
        echo -e "${RED}✗ FAILED${NC} [HTTP Status: $actual_status, Expected: $expected_status]"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        record_result "$test_id" "$test_desc" "$expected_status" "$actual_status" "FAIL"
    fi
    echo ""
}

print_results_table() {
    echo ""
    echo -e "${CYAN}┌─────────────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│                         TEST RESULTS SUMMARY                                │${NC}"
    echo -e "${CYAN}├──────────┬────────────────────────────────┬──────────┬──────────┬──────────┤${NC}"
    echo -e "${CYAN}│ Test ID  │ Description                    │ Expected │ Actual   │ Result   │${NC}"
    echo -e "${CYAN}├──────────┼────────────────────────────────┼──────────┼──────────┼──────────┤${NC}"
    
    for i in "${!TEST_IDS[@]}"; do
        local result_color="${GREEN}"
        if [ "${TEST_RESULTS[$i]}" = "FAIL" ]; then
            result_color="${RED}"
        fi
        printf "${CYAN}│${NC} %-8s ${CYAN}│${NC} %-30s ${CYAN}│${NC} %-8s ${CYAN}│${NC} %-8s ${CYAN}│${NC} ${result_color}%-8s${NC} ${CYAN}│${NC}\n" \
            "${TEST_IDS[$i]}" \
            "${TEST_DESCRIPTIONS[$i]:0:30}" \
            "${EXPECTED_STATUS[$i]}" \
            "${ACTUAL_STATUS[$i]}" \
            "${TEST_RESULTS[$i]}"
    done
    
    echo -e "${CYAN}└──────────┴────────────────────────────────┴──────────┴──────────┴──────────┘${NC}"
}

# -----------------------------------------------------------------------------
# TEST CASES
# -----------------------------------------------------------------------------

# AC1.1: Missing API Key
TEST_ID="AC1.1"
TEST_DESC="Missing API Key"
EXPECTED="401"
TESTS_TOTAL=$((TESTS_TOTAL + 1))
print_test_header "$TEST_ID: $TEST_DESC"
print_expected "HTTP $EXPECTED"
print_result
HTTP_CODE=$(curl -sk -o /tmp/response.txt -w "%{http_code}" -X POST "${BASE_URL}/api/downloads" \
    -H "Content-Type: application/json" \
    -m 10 \
    -d '{"url": "https://speed.hetzner.de/100MB.bin"}')
BODY=$(cat /tmp/response.txt)
check_result "$TEST_ID" "$TEST_DESC" "$EXPECTED" "$HTTP_CODE" "$BODY"

# AC1.2: Invalid API Key
TEST_ID="AC1.2"
TEST_DESC="Invalid API Key"
EXPECTED="401"
TESTS_TOTAL=$((TESTS_TOTAL + 1))
print_test_header "$TEST_ID: $TEST_DESC"
print_expected "HTTP $EXPECTED"
print_result
HTTP_CODE=$(curl -sk -o /tmp/response.txt -w "%{http_code}" -X POST "${BASE_URL}/api/downloads" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: invalid-fake-key" \
    -m 10 \
    -d '{"url": "https://speed.hetzner.de/100MB.bin"}')
BODY=$(cat /tmp/response.txt)
check_result "$TEST_ID" "$TEST_DESC" "$EXPECTED" "$HTTP_CODE" "$BODY"

# AC1.3: Missing URL
TEST_ID="AC1.3"
TEST_DESC="Missing URL"
EXPECTED="400"
TESTS_TOTAL=$((TESTS_TOTAL + 1))
print_test_header "$TEST_ID: $TEST_DESC"
print_expected "HTTP $EXPECTED"
print_result
HTTP_CODE=$(curl -sk -o /tmp/response.txt -w "%{http_code}" -X POST "${BASE_URL}/api/downloads" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -m 10 \
    -d '{}')
BODY=$(cat /tmp/response.txt)
check_result "$TEST_ID" "$TEST_DESC" "$EXPECTED" "$HTTP_CODE" "$BODY"

# AC2.1: Enqueue Download
TEST_ID="AC2.1"
TEST_DESC="Enqueue Download"
EXPECTED="201"
TESTS_TOTAL=$((TESTS_TOTAL + 1))
print_test_header "$TEST_ID: $TEST_DESC"
print_expected "HTTP $EXPECTED"
print_result
HTTP_CODE=$(curl -sk -o /tmp/response.txt -w "%{http_code}" -X POST "${BASE_URL}/api/downloads" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -m 10 \
    -d '{"url": "https://speed.hetzner.de/100MB.bin", "target_clients": ["pc-da-sala"]}')
BODY=$(cat /tmp/response.txt)
check_result "$TEST_ID" "$TEST_DESC" "$EXPECTED" "$HTTP_CODE" "$BODY"


# -----------------------------------------------------------------------------
# CLEANUP
# -----------------------------------------------------------------------------
rm -f /tmp/response.txt

# -----------------------------------------------------------------------------
# TEST SUMMARY
# -----------------------------------------------------------------------------
echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}TEST EXECUTION COMPLETE${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Base URL: ${BASE_URL}"
echo "Finished at: $(date)"
echo ""

print_results_table

echo ""
echo -e "Tests Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "Tests Failed: ${RED}${TESTS_FAILED}${NC}"
echo -e "Total Tests:  ${TESTS_TOTAL}"
echo ""

if [ "$TESTS_TOTAL" -gt 0 ]; then
    PASS_RATE=$((TESTS_PASSED * 100 / TESTS_TOTAL))
    if [ "$TESTS_FAILED" -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed! (${PASS_RATE}%)${NC}"
    else
        echo -e "${RED}✗ Some tests failed (${PASS_RATE}% passed)${NC}"
    fi
fi
echo ""

if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
fi
