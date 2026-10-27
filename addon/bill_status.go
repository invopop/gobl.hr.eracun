package eracun

import (
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"
)

func normalizeStatus(st *bill.Status) {
	if st == nil {
		return
	}
	normalizePartyEndpoint(st.Supplier)
	normalizePartyEndpoint(st.Customer)
}

// billStatusRules cover the buyer's rejection of an invoice, the only
// application response the HR specification requires.
func billStatusRules() *rules.Set {
	return rules.For(new(bill.Status),
		rules.Field("supplier",
			rules.Assert("01", "status supplier requires an eRačun address registered in AMS: a Croatian tax ID or an ISO 6523 endpoint, not an email (AR-BT-8)",
				is.Func("has endpoint", partyHasEndpoint),
			),
		),
		rules.Field("customer",
			rules.Assert("02", "status customer is required", is.Present),
			rules.Assert("03", "status customer requires an eRačun address registered in AMS: a Croatian tax ID or an ISO 6523 endpoint, not an email (AR-BT-6)",
				is.Func("has endpoint", partyHasEndpoint),
			),
		),
		rules.Field("lines",
			rules.Each(
				rules.Field("key",
					rules.Assert("04", "status line key must be rejected (AR-BT-10)", is.In(bill.StatusLineRejected)),
				),
				rules.Assert("05", "status line requires a description of the rejection, on the line or its reason (AR-BT-11)",
					is.Func("has description", statusLineHasDescription),
				),
				rules.Field("reasons",
					rules.Assert("06", "status line requires one reason with a hr-eracun-rejection-reason extension (AR-BT-12)",
						is.Func("has one rejection reason", reasonsHaveOneRejectionReason),
					),
				),
				rules.Field("doc",
					rules.Assert("07", "status line requires the rejected invoice (AR-BG-6)", is.Present),
					rules.Field("code",
						rules.Assert("08", "status line doc code is required (AR-BT-13)", is.Present),
					),
					rules.Field("issue_date",
						rules.Assert("09", "status line doc issue date is required (AR-BT-14)", is.Present),
					),
				),
			),
		),
	)
}

func statusLineHasDescription(val any) bool {
	line, ok := val.(*bill.StatusLine)
	if !ok || line == nil || line.Description != "" {
		return true
	}
	for _, r := range line.Reasons {
		if r != nil && r.Description != "" {
			return true
		}
	}
	return false
}

func reasonsHaveOneRejectionReason(val any) bool {
	reasons, _ := val.([]*bill.Reason)
	return len(RejectionReasons(reasons)) == 1
}

// RejectionReasons provides the reasons that carry a rejection reason code.
func RejectionReasons(reasons []*bill.Reason) []*bill.Reason {
	var out []*bill.Reason
	for _, r := range reasons {
		if r != nil && r.Ext.Has(ExtKeyRejectionReason) {
			out = append(out, r)
		}
	}
	return out
}

// reasonRules validate the rejection reason codes.
func reasonRules() *rules.Set {
	return rules.For(new(bill.Reason),
		rules.Field("ext",
			rules.Assert("01", "reason hr-eracun-rejection-reason must be N, U or O",
				tax.ExtensionHasValidCode(ExtKeyRejectionReason),
			),
		),
	)
}
