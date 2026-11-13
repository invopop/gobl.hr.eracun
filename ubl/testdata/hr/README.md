# HR CIUS examples

The 20 official eRačun examples of the Tax Administration ("Primjeri eRačuna"),
with three changes so that they pass the HR rules (`hr.gov.porezna.eracun:*:1.0.4`).
As published, every file fails the HR schematron:

| Change | Rule |
|---|---|
| The seller and operator OIB `12345678901`, which fails the mod 11,10 check, is replaced by `12345678903` | HR-BR-9, HR-BR-53 |
| Every 2025 date is moved to 2026 | HR-BR-40 (issue date on or after 2026-01-01) |
| `Racun za leasing PDV PPMV-aktivacija.xml` gets the KPD `77.11.01` on its only line | HR-BR-25 |

`out/` holds each example imported into GOBL (`.json`) and exported back to UBL
(`.xml`). Regenerate them with `go test ./ubl/ -update`.
