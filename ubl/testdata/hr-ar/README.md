# HR application response example

`odbijanje.xml` is the rejection example of the Tax Administration's
application response specification (AplikacijskiOdgovor v1.1, section 4), with
one change: the two OIBs of the published example are replaced by the
placeholder OIBs of the invoice examples in `../hr`.

| Party | Becomes |
|---|---|
| Sender (the buyer who rejects) | `11111111119` |
| Receiver (the seller), in `EndpointID` and `PartyIdentification` | `12345678903` |

`out/` holds the example imported into GOBL (`.json`) and exported back to UBL
(`.xml`). Regenerate them with `go test ./ubl/ -update`.
