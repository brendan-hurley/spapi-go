// ABOUTME: Smoke test program that hits the SP-API sandbox using the LWA
// ABOUTME: RoundTripper. Exercises Orders, Listings Items, and Finances.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/brendan-hurley/spapi-go/auth"

	financesv0 "github.com/brendan-hurley/spapi-go/apis/financesv0"
	listings "github.com/brendan-hurley/spapi-go/apis/listingsitems20210801"
	ordersv0 "github.com/brendan-hurley/spapi-go/apis/ordersv0"
)

// Sandbox base URL for North America.
const sandboxURL = "https://sandbox.sellingpartnerapi-na.amazon.com"

// Marketplace id for Amazon.com (US).
const usMarketplaceID = "ATVPDKIKX0DER"

func main() {
	// Credentials are sourced from env vars so we don't bake them into
	// source. The caller should export them from the Postman env file
	// before running:
	//   SPAPI_CLIENT_ID, SPAPI_CLIENT_SECRET, SPAPI_REFRESH_TOKEN, SPAPI_SELLER_ID
	creds := auth.Credentials{
		ClientID:     mustEnv("SPAPI_CLIENT_ID"),
		ClientSecret: mustEnv("SPAPI_CLIENT_SECRET"),
		RefreshToken: mustEnv("SPAPI_REFRESH_TOKEN"),
	}
	sellerID := mustEnv("SPAPI_SELLER_ID")

	lwa := auth.NewClient(creds)
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: auth.NewRoundTripper(lwa, nil),
	}
	ctx := context.Background()

	// --- Orders API (ordersV0) ---
	// Sandbox magic: CreatedAfter="TEST_CASE_200" returns a canned order list.
	{
		cfg := ordersv0.NewConfiguration()
		cfg.HTTPClient = httpClient
		cfg.Servers = ordersv0.ServerConfigurations{{URL: sandboxURL}}
		client := ordersv0.NewAPIClient(cfg)

		resp, httpResp, err := client.OrdersV0API.GetOrders(ctx).
			MarketplaceIds([]string{usMarketplaceID}).
			CreatedAfter("TEST_CASE_200").
			Execute()
		report("orders.GetOrders", httpResp, err, func() string {
			if resp == nil || resp.Payload == nil {
				return "no payload"
			}
			orders := resp.Payload.Orders
			return fmt.Sprintf("%d orders returned", len(orders))
		})
	}

	// --- Listings Items API (listings_2021-08-01) ---
	// Sandbox returns the canned "Hardside Carry-On Spinner Suitcase" item
	// for any sku/sellerId combination.
	{
		cfg := listings.NewConfiguration()
		cfg.HTTPClient = httpClient
		cfg.Servers = listings.ServerConfigurations{{URL: sandboxURL}}
		client := listings.NewAPIClient(cfg)

		resp, httpResp, err := client.ListingsAPI.
			GetListingsItem(ctx, sellerID, "GM-ZDPI-9B4E").
			MarketplaceIds([]string{usMarketplaceID}).
			Execute()
		report("listings.GetListingsItem", httpResp, err, func() string {
			if resp == nil {
				return "no payload"
			}
			return fmt.Sprintf("sku=%s, summaries=%d, offers=%d",
				resp.GetSku(), len(resp.Summaries), len(resp.Offers))
		})
	}

	// --- Finances API (financesV0) ---
	// Sandbox matches query params *literally*. ListFinancialEventGroups
	// takes date-time params that Go serializes as RFC3339 but the
	// sandbox expects "2019-10-13" — use ListFinancialEventsByGroupId
	// instead which just needs a path param.
	{
		cfg := financesv0.NewConfiguration()
		cfg.HTTPClient = httpClient
		cfg.Servers = financesv0.ServerConfigurations{{URL: sandboxURL}}
		client := financesv0.NewAPIClient(cfg)

		resp, httpResp, err := client.DefaultAPI.
			ListFinancialEventsByGroupId(ctx, "485734534857").
			MaxResultsPerPage(10).
			Execute()
		report("finances.ListFinancialEventsByGroupId", httpResp, err, func() string {
			if resp == nil || resp.Payload == nil || resp.Payload.FinancialEvents == nil {
				return "no payload"
			}
			ev := resp.Payload.FinancialEvents
			return fmt.Sprintf("shipments=%d, refunds=%d, fees=%d",
				len(ev.ShipmentEventList), len(ev.RefundEventList), len(ev.ServiceFeeEventList))
		})
	}
}

// report prints a one-line summary for each API call. bodyDesc is a
// closure so we only touch resp fields on success (they're nil on error).
func report(name string, httpResp *http.Response, err error, bodyDesc func() string) {
	if err != nil {
		status := "no http resp"
		body := ""
		if httpResp != nil {
			status = httpResp.Status
			b, _ := io.ReadAll(httpResp.Body)
			body = string(b)
			if len(body) > 200 {
				body = body[:200] + "..."
			}
		}
		fmt.Printf("[FAIL] %-38s %s: %v\n       body: %s\n", name, status, err, body)
		return
	}
	fmt.Printf("[ OK ] %-38s %s | %s\n", name, httpResp.Status, bodyDesc())
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required env var: %s\n", key)
		os.Exit(2)
	}
	return v
}
