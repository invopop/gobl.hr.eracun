# HR application response example

`odbijanje.xml` is the rejection example of the Tax Administration's
application response specification (AplikacijskiOdgovor v1.1, section 4), with
one change: the two OIBs of the published example are replaced by the
placeholder OIBs of the invoice examples in `../hr`.

| Party | Becomes |
|---|---|
| Sender (the buyer who rejects) | `11111111119` |
| Receiver (the seller), in `EndpointID` and `PartyIdentification` | `12345678903` |

`prihvacanje.xml` is the same example sent as an acceptance, which the
specification allows by agreement between the parties: the response code is
`AP` instead of `RE`, and the rejection's description and reason are left out.

`upit.xml` is the same example sent as a query (`UQ`): the description says
what is missing, and the one status carries the Peppol status reason `REF`
(`OPStatusReason`), as the parties may agree (AR-BG-5).

`out/` holds each example imported into GOBL (`.json`) and exported back to UBL
(`.xml`). Regenerate them with `go test ./ubl/ -update`.
