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
	responseCodeRejected  = "RE"
	rejectionReasonListID = "MFINVrstaRazlogaOdbijanja"
)

// SpecificationApplicationResponse is the application response a buyer sends
// to reject an invoice. No validation artefact covers it beyond the UBL 2.1
// schema.
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
// rejection.
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
		if line.Key == bill.StatusLineRejected {
			listID := responseCodeListID
			dr.Response.ResponseCode = &goblubl.IDType{ListID: &listID, Value: responseCodeRejected}
		}
		for _, r := range eracun.RejectionReasons(line.Reasons) {
			listID := rejectionReasonListID
			dr.Response.Status = append(dr.Response.Status, &goblubl.Status{
				StatusReasonCode: &goblubl.IDType{
					ListID: &listID,
					Value:  r.Ext.Get(eracun.ExtKeyRejectionReason).String(),
				},
			})
		}
	}
	return nil
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

// importApplicationResponse reads the HR codes of an imported rejection.
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
		if rc := dr.Response.ResponseCode; rc != nil && rc.Value == responseCodeRejected {
			line.Key = bill.StatusLineRejected
		}
		for _, s := range dr.Response.Status {
			if s == nil || s.StatusReasonCode == nil || s.StatusReasonCode.ListID == nil || *s.StatusReasonCode.ListID != rejectionReasonListID {
				continue
			}
			line.Reasons = append(line.Reasons, &bill.Reason{
				Key: bill.ReasonKeyOther,
				Ext: tax.ExtensionsOf(cbc.CodeMap{
					eracun.ExtKeyRejectionReason: cbc.Code(s.StatusReasonCode.Value),
				}),
			})
		}
	}
	return nil
}
