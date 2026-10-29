package eracun

import (
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
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

// otherResponseKeys are the status line keys of the Peppol invoice response
// codes besides the rejection, which the HR application response allows only
// by agreement between the supplier and the buyer (AR-BT-10).
var otherResponseKeys = []cbc.Key{
	bill.StatusLineAccepted,
	bill.StatusLineAcknowledged,
	bill.StatusLineProcessing,
	bill.StatusLineQuerying,
	bill.StatusLinePaid,
}

// billStatusRules cover the application responses a buyer sends about an
// invoice. Every intermediary must support the rejection, the only response
// with a rejection reason. A response answers one invoice and has at most one
// status (AR-BG-5): the rejection reason, or one agreed Peppol status reason
// or action.
func billStatusRules() *rules.Set {
	return rules.For(new(bill.Status),
		rules.Field("type",
			rules.Assert("13", "status type must be response: an HR application response goes from the buyer to the supplier (AR-BG-2, AR-BG-3)",
				is.In(bill.StatusTypeResponse),
			),
		),
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
			rules.Assert("14", "status requires exactly one line: an HR application response answers one invoice (AR-BG-4, AR-BG-6)",
				is.Length(1, 1),
			),
			rules.Each(
				rules.Field("key",
					rules.Assert("04", "status line key must be rejected, accepted, acknowledged, processing, querying or paid (AR-BT-10)",
						is.In(
							bill.StatusLineRejected,
							bill.StatusLineAccepted,
							bill.StatusLineAcknowledged,
							bill.StatusLineProcessing,
							bill.StatusLineQuerying,
							bill.StatusLinePaid,
						),
					),
				),
				rules.When(bill.StatusLineKeyIn(bill.StatusLineRejected),
					rules.Assert("05", "status line requires a description of the rejection, on the line or its reason (AR-BT-11)",
						is.Func("has description", statusLineHasDescription),
					),
					rules.Field("reasons",
						rules.Assert("06", "status line requires one reason with a hr-eracun-rejection-reason extension (AR-BT-12)",
							is.Func("has one rejection reason", reasonsHaveOneRejectionReason),
						),
					),
				),
				rules.Assert("11", "status line can have at most one reason or action, the one status of an HR application response (AR-BG-5)",
					is.Func("has at most one status", statusLineHasAtMostOneStatus),
				),
				// The other responses may carry one Peppol status reason or action
				// agreed between the parties, as a code without text.
				rules.When(bill.StatusLineKeyIn(otherResponseKeys...),
					rules.Field("reasons",
						rules.Assert("10", "status line reasons of a response other than a rejection cannot have a hr-eracun-rejection-reason extension (AR-BG-5)",
							is.Func("has no rejection reason", reasonsHaveNoRejectionReason),
						),
					),
					rules.Assert("12", "status line reason or action of a response other than a rejection has no text in an HR application response; the text goes in the line description (AR-BT-11, AR-BG-5)",
						is.Func("has no status text", statusLineHasNoStatusText),
					),
				),
				rules.Field("doc",
					rules.Assert("07", "status line requires the invoice it responds to (AR-BG-6)", is.Present),
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

func statusLineHasAtMostOneStatus(val any) bool {
	line, ok := val.(*bill.StatusLine)
	return !ok || line == nil || len(line.Reasons)+len(line.Actions) <= 1
}

func statusLineHasNoStatusText(val any) bool {
	line, ok := val.(*bill.StatusLine)
	if !ok || line == nil {
		return true
	}
	for _, r := range line.Reasons {
		if r != nil && r.Description != "" {
			return false
		}
	}
	for _, a := range line.Actions {
		if a != nil && a.Description != "" {
			return false
		}
	}
	return true
}

func reasonsHaveOneRejectionReason(val any) bool {
	reasons, _ := val.([]*bill.Reason)
	return len(RejectionReasons(reasons)) == 1
}

func reasonsHaveNoRejectionReason(val any) bool {
	reasons, _ := val.([]*bill.Reason)
	return len(RejectionReasons(reasons)) == 0
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
