package nmockapi

import (
	context "context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ngrok/ngrok-api-go/v7"
)

type IPPolicyRuleClient struct {
	baseClient[*ngrok.IPPolicyRule]
	ippClient *IPPolicyClient
}

// NewIPPolicyRuleClient constructs a mock IP policy rule client used by tests.
func NewIPPolicyRuleClient(ippClient *IPPolicyClient) *IPPolicyRuleClient {
	return &IPPolicyRuleClient{
		baseClient: newBase[*ngrok.IPPolicyRule](
			"ippr",
		),
		ippClient: ippClient,
	}
}

func (m *IPPolicyRuleClient) Create(ctx context.Context, item *ngrok.IPPolicyRuleCreate) (*ngrok.IPPolicyRule, error) {
	if item.Action == nil {
		return nil, &ngrok.Error{
			StatusCode: http.StatusBadRequest,
			Msg:        "Missing action",
			ErrorCode:  "ERR_NGROK_400",
		}
	}
	if *item.Action != "allow" && *item.Action != "deny" {
		return nil, &ngrok.Error{
			StatusCode: http.StatusBadRequest,
			Msg:        fmt.Sprintf("Invalid action: %s", *item.Action),
			ErrorCode:  "ERR_NGROK_400",
		}
	}
	if !isValidCIDR(item.CIDR) {
		return nil, &ngrok.Error{
			StatusCode: http.StatusBadRequest,
			Msg:        fmt.Sprintf("Invalid CIDR: %s", item.CIDR),
			ErrorCode:  "ERR_NGROK_400",
		}
	}

	id := m.newID()

	newRule := &ngrok.IPPolicyRule{
		ID:          id,
		CreatedAt:   m.createdAt(),
		Action:      *item.Action,
		CIDR:        item.CIDR,
		Description: item.Description,
	}

	m.items[id] = newRule
	return newRule, nil
}

func (m *IPPolicyRuleClient) Update(ctx context.Context, item *ngrok.IPPolicyRuleUpdate) (*ngrok.IPPolicyRule, error) {
	existingItem, err := m.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	if item.CIDR != nil {
		if !isValidCIDR(*item.CIDR) {
			return nil, &ngrok.Error{
				StatusCode: http.StatusBadRequest,
				Msg:        fmt.Sprintf("Invalid CIDR: %s", *item.CIDR),
				ErrorCode:  "ERR_NGROK_400",
			}
		}
		existingItem.CIDR = *item.CIDR
	}
	if item.Description != nil {
		existingItem.Description = *item.Description
	}
	// The ngrok SDK's IPPolicyRuleUpdate type does not include Action/Priority fields
	// as separate values here; mocks only update fields that exist on the real type.

	m.items[item.ID] = existingItem
	return existingItem, nil
}

func isValidCIDR(cidr string) bool {
	// Basic validation for CIDR format
	if strings.Count(cidr, "/") != 1 {
		return false
	}
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return false
	}
	ipPart := parts[0]
	prefixPart := parts[1]

	ipSegments := strings.Split(ipPart, ".")
	if len(ipSegments) != 4 {
		return false
	}
	for _, segment := range ipSegments {
		if segment == "" {
			return false
		}
		num := 0
		for _, ch := range segment {
			if ch < '0' || ch > '9' {
				return false
			}
			num = num*10 + int(ch-'0')
		}
		if num < 0 || num > 255 {
			return false
		}
	}

	prefixNum := 0
	for _, ch := range prefixPart {
		if ch < '0' || ch > '9' {
			return false
		}
		prefixNum = prefixNum*10 + int(ch-'0')
	}
	if prefixNum < 0 || prefixNum > 32 {
		return false
	}

	return true
}
