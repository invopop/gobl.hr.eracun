package ubl

import (
	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/tax"
)

// IDApplicationResponse identifies the HR application response, as both its
// CustomizationID and ProfileID (AR-BT-1, AR-BT-2).
const IDApplicationResponse = "urn:mfin.gov.hr:AplikacijskiOdgovor-2025:1.0"

// Code lists of the HR application response.
const (
	responseCodeListID    = "UNCL4343OpSubset"
	rejectionReasonListID = "MFINVrstaRazlogaOdbijanja"
	statusReasonListID    = "OPStatusReason"
	statusActionListID    = "OPStatusAction"

	// statusCodeOther is "other" in both Peppol status clarification lists.
	statusCodeOther = "OTH"
)

// statusReasonCodes and statusActionCodes map the reason and action keys of
// a response other than a rejection to the Peppol status clarifications
// (OPStatusReason, OPStatusAction), which the parties may agree to use
// (AR-BG-5). They are the lists gobl.ubl maps for the Peppol invoice response,
// which it does not export.
var statusReasonCodes = map[cbc.Key]string{
	bill.ReasonKeyNone:            "NON",
	bill.ReasonKeyReferences:      "REF",
	bill.ReasonKeyLegal:           "LEG",
	bill.ReasonKeyUnknownReceiver: "REC",
	bill.ReasonKeyQuality:         "QUA",
	bill.ReasonKeyDelivery:        "DEL",
	bill.ReasonKeyPrices:          "PRI",
	bill.ReasonKeyQuantity:        "QTY",
	bill.ReasonKeyItems:           "ITM",
	bill.ReasonKeyPaymentTerms:    "PAY",
	bill.ReasonKeyNotRecognized:   "UNR",
	bill.ReasonKeyFinanceTerms:    "FIN",
	bill.ReasonKeyPartial:         "PPD",
	bill.ReasonKeyOther:           statusCodeOther,
}

var statusActionCodes = map[cbc.Key]string{
	bill.ActionKeyNone:          "NOA",
	bill.ActionKeyProvide:       "PIN",
	bill.ActionKeyReissue:       "NIN",
	bill.ActionKeyCreditFull:    "CNF",
	bill.ActionKeyCreditPartial: "CNP",
	bill.ActionKeyCreditAmount:  "CNA",
	bill.ActionKeyOther:         statusCodeOther,
}

// responseCodes maps the status line keys to the Peppol invoice response codes
// (UNCL4343 subset, T111) that the HR specification uses (AR-BT-10).
var responseCodes = map[cbc.Key]string{
	bill.StatusLineAcknowledged: "AB",
	bill.StatusLineProcessing:   "IP",
	bill.StatusLineQuerying:     "UQ",
	bill.StatusLineRejected:     "RE",
	bill.StatusLineAccepted:     "AP",
	bill.StatusLinePaid:         "PD",
}

// responseKeys reverses responseCodes for import. A conditional acceptance
// (CA) is read as accepted, as gobl.ubl reads it for Peppol, but its
// conditions are not read.
var responseKeys = map[string]cbc.Key{
	"AB": bill.StatusLineAcknowledged,
	"IP": bill.StatusLineProcessing,
	"UQ": bill.StatusLineQuerying,
	"RE": bill.StatusLineRejected,
	"AP": bill.StatusLineAccepted,
	"PD": bill.StatusLinePaid,
	"CA": bill.StatusLineAccepted,
}

// SpecificationApplicationResponse is the application response a buyer sends
// about an invoice: a rejection, or by agreement with the supplier another
// code of the Peppol invoice response. No validation artefact covers it beyond
// the UBL 2.1 schema.
var SpecificationApplicationResponse = goblubl.Specification{
	CustomizationID: IDApplicationResponse,
	ProfileID:       IDApplicationResponse,
	Addons:          []cbc.Key{eracun.V1},
	Applies: func(doc any) bool {
		_, ok := doc.(*bill.Status)
		return ok
	},
}

// exportApplicationResponse applies the HR data model to an exported
// application response.
func exportApplicationResponse(_ *goblubl.Format, env *gobl.Envelope, doc goblubl.Document) error {
	st, ok := env.Extract().(*bill.Status)
	out, ok2 := doc.(*goblubl.ApplicationResponse)
	if !ok || !ok2 {
		return nil
	}
	trimResponseParty(out.SenderParty)
	trimResponseParty(out.ReceiverParty)
	for i, dr := range out.DocumentResponse {
		if i >= len(st.Lines) || dr == nil || dr.Response == nil {
			continue
		}
		line := st.Lines[i]
		if code, ok := responseCodes[line.Key]; ok {
			listID := responseCodeListID
			dr.Response.ResponseCode = &goblubl.IDType{ListID: &listID, Value: code}
		}
		if line.Key == bill.StatusLineRejected {
			for _, r := range eracun.RejectionReasons(line.Reasons) {
				dr.Response.Status = append(dr.Response.Status,
					responseStatus(rejectionReasonListID, r.Ext.Get(eracun.ExtKeyRejectionReason).String(), ""),
				)
			}
			continue
		}
		// gobl.ubl falls back to a reason's text for the description, which
		// only a rejection requires; the reason carries it here.
		if line.Description == "" {
			dr.Response.Description = nil
		}
		for _, r := range line.Reasons {
			if code, ok := statusReasonCodes[r.Key]; ok {
				dr.Response.Status = append(dr.Response.Status, responseStatus(statusReasonListID, code, r.Description))
			}
		}
		for _, a := range line.Actions {
			if code, ok := statusActionCodes[a.Key]; ok {
				dr.Response.Status = append(dr.Response.Status, responseStatus(statusActionListID, code, a.Description))
			}
		}
	}
	return nil
}

func responseStatus(listID, code, reason string) *goblubl.Status {
	s := &goblubl.Status{StatusReasonCode: &goblubl.IDType{ListID: &listID, Value: code}}
	if reason != "" {
		s.StatusReason = []string{reason}
	}
	return s
}

// trimResponseParty keeps the party elements of the HR data model: the
// electronic address and the identifier (AR-BG-2, AR-BG-3).
func trimResponseParty(p *goblubl.Party) {
	if p == nil {
		return
	}
	*p = goblubl.Party{
		EndpointID:          p.EndpointID,
		PartyIdentification: p.PartyIdentification,
	}
}

// importApplicationResponse reads the HR codes of an imported application
// response.
func importApplicationResponse(_ *goblubl.Format, doc goblubl.Document, env *gobl.Envelope) error {
	in, ok := doc.(*goblubl.ApplicationResponse)
	st, ok2 := env.Extract().(*bill.Status)
	if !ok || !ok2 {
		return nil
	}
	for i, dr := range in.DocumentResponse {
		if i >= len(st.Lines) || dr == nil || dr.Response == nil {
			continue
		}
		line := st.Lines[i]
		if rc := dr.Response.ResponseCode; rc != nil {
			line.Key = responseKeys[rc.Value]
		}
		for _, s := range dr.Response.Status {
			if s == nil || s.StatusReasonCode == nil || s.StatusReasonCode.ListID == nil {
				continue
			}
			code := s.StatusReasonCode.Value
			switch *s.StatusReasonCode.ListID {
			case rejectionReasonListID:
				line.Reasons = append(line.Reasons, &bill.Reason{
					Key: bill.ReasonKeyOther,
					Ext: tax.ExtensionsOf(cbc.CodeMap{
						eracun.ExtKeyRejectionReason: cbc.Code(code),
					}),
				})
			case statusReasonListID:
				// A rejection carries only its rejection reason.
				if key := keyForCode(statusReasonCodes, code); key != "" && line.Key != bill.StatusLineRejected {
					line.Reasons = append(line.Reasons, &bill.Reason{Key: key, Description: statusReason(s)})
				}
			case statusActionListID:
				if key := keyForCode(statusActionCodes, code); key != "" && line.Key != bill.StatusLineRejected {
					line.Actions = append(line.Actions, &bill.Action{Key: key, Description: statusReason(s)})
				}
			}
		}
	}
	return nil
}

// keyForCode provides the key mapped to code in m, or empty if none is.
func keyForCode(m map[cbc.Key]string, code string) cbc.Key {
	for k, v := range m {
		if v == code {
			return k
		}
	}
	return ""
}

func statusReason(s *goblubl.Status) string {
	if len(s.StatusReason) == 0 {
		return ""
	}
	return s.StatusReason[0]
}
