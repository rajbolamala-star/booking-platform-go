#!/usr/bin/env bash
# Quick smoke tests for the booking platform.
# Run after `make docker-up`.

set -e

GATEWAY="${GATEWAY:-http://localhost:8080}"

echo "=== 1. Health checks ==="
curl -sS "$GATEWAY/health" | jq .
curl -sS "http://localhost:8081/health" | jq .
curl -sS "http://localhost:8082/health" | jq .
curl -sS "http://localhost:8083/health" | jq .

echo
echo "=== 2. Login (get JWT) ==="
TOKEN=$(curl -sS -X POST "$GATEWAY/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"u123","email":"test@example.com"}' | jq -r .token)
echo "Token: $TOKEN"

echo
echo "=== 3. Search ==="
curl -sS -G "$GATEWAY/api/search/search" \
  --data-urlencode "location=Seattle" \
  -H "Authorization: Bearer $TOKEN" | jq .

echo
echo "=== 4. Get price ==="
curl -sS -X POST "$GATEWAY/api/price/price" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "property_id":"p1",
    "check_in":"2025-06-01T00:00:00Z",
    "check_out":"2025-06-05T00:00:00Z",
    "guests":2
  }' | jq .

echo
echo "=== 5. Create reservation ==="
curl -sS -X POST "$GATEWAY/api/reservations/reservations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id":"u123",
    "property_id":"p1",
    "check_in":"2025-06-01T00:00:00Z",
    "check_out":"2025-06-05T00:00:00Z",
    "guests":2
  }' | jq .

echo
echo "=== Done ==="
