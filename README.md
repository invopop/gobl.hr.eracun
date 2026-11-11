# GOBL ➡️ Croatia eRačun

Croatian eRačun addon and UBL format for [GOBL](https://github.com/invopop/gobl).

Released under the Apache 2.0 [LICENSE](https://github.com/invopop/gobl.hr.eracun/blob/main/LICENSE), Copyright 2026 [Invopop S.L.](https://invopop.com).

[![Lint](https://github.com/invopop/gobl.hr.eracun/actions/workflows/lint.yaml/badge.svg)](https://github.com/invopop/gobl.hr.eracun/actions/workflows/lint.yaml)
[![Test Go](https://github.com/invopop/gobl.hr.eracun/actions/workflows/test.yaml/badge.svg)](https://github.com/invopop/gobl.hr.eracun/actions/workflows/test.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/invopop/gobl.hr.eracun)](https://goreportcard.com/report/github.com/invopop/gobl.hr.eracun)
[![codecov](https://codecov.io/gh/invopop/gobl.hr.eracun/graph/badge.svg)](https://codecov.io/gh/invopop/gobl.hr.eracun)
[![GoDoc](https://godoc.org/github.com/invopop/gobl.hr.eracun?status.svg)](https://godoc.org/github.com/invopop/gobl.hr.eracun)
![Latest Tag](https://img.shields.io/github/v/tag/invopop/gobl.hr.eracun)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/invopop/gobl.hr.eracun)

This module implements the Croatian eRačun, the electronic invoice that
domestic businesses exchange under Fiscalization 2.0 (Zakon o fiskalizaciji,
NN 89/25). An eRačun follows EN 16931 with the HR CIUS 2025 specification and
its national extension (CustomizationID
`urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0`).
The module provides:

- the GOBL addon `hr-eracun-v1`, which requires `eu-en16931-v2017`;
- the UBL format `ubl+hr-eracun-v1` for invoices and credit notes, validated
  with the Tax Administration's rules `hr.gov.porezna.eracun:ubl-invoice:1.0.4`
  and `hr.gov.porezna.eracun:ubl-creditnote:1.0.4`;
- the application response a buyer sends about an eRačun
  (`urn:mfin.gov.hr:AplikacijskiOdgovor-2025:1.0`), mapped to `bill.Status`:
  the rejection, and by agreement between the parties the other Peppol invoice
  response codes.

The module also fiscalizes invoices that are not eRačuni, such as those a
shop issues at the till (Fiscalization 1.0, "fiskalizacija računa u krajnjoj
potrošnji"), with the Tax Administration's invoice fiscalization service:
the GOBL addon `hr-fiskal-b2c-v1` in `addon/fiskal/`, and the conversion and
client in `racun/`. Croatian law calls these documents računi (invoices), the same word
as in eRačun; it has no separate term for a receipt.

The Croatian tax regime itself (`regimes/hr`) lives in GOBL core. This module
carries only what Croatian fiscalization adds on top: the eRačun addon and
format, and the `racun` client, so that only projects handling Croatian
invoices take on their weight.

## Layout

- `addon/` — the GOBL addon: extensions, normalizers and validation rules that
  register into GOBL on import. This package is kept dependency-light so
  importing it never pulls in conversion tooling.
- `ubl/` — the HR CIUS UBL format, built on the base import and export of
  [gobl.ubl](https://github.com/invopop/gobl.ubl) and registered with gobl.ubl
  and GOBL's `convert` register on import. Export writes the business process,
  the HR tax categories, the operator (`SellerContact`) and the
  `hrextac:HRFISK20Data` extension; import reads the process, operator, HR
  tax categories and cash-basis flag back into GOBL.
- `addon/fiskal/` — the GOBL addon `hr-fiskal-b2c-v1` for invoices
  fiscalized at the time of the sale: the business premises and device
  labels, the numbering scope, the VAT system and the paper invoice number as
  extensions of the invoice tax, the operator as a supplier person (as in
  `hr-eracun-v1`), and the rules CIS needs met. It also defines the stamps of
  a fiscalized invoice: `fiskal-zki`, `fiskal-jir` and `fiskal-qr`.
- `racun/` — invoice fiscalization: builds the invoice request
  (`RacunZahtjev`) from a GOBL invoice with the `hr-fiskal-b2c-v1` addon,
  computes its ZKI (the issuer's protection code), signs it with XML-DSig,
  and sends it to CIS, checking CIS's signed answer for the JIR. `AddStamps`
  adds the ZKI, the JIR and the QR code URL to the envelope.
- `ca/` — the FINA CA certificates that CIS clients trust, embedded: the demo
  chain for CIS test and the production chain for production CIS. FINA's CAs
  are not in the system certificate store, and the CIS servers send their
  TLS certificate without its intermediate. `racun` trusts the chain of its
  environment and nothing else.
- the module root is reserved for other eRačun tooling built on top of the
  addon.

## Usage

Add a blank import of the **addon** so it registers itself, then use GOBL as
normal:

```go
import (
	"github.com/invopop/gobl"
	_ "github.com/invopop/gobl.hr.eracun/addon"
)
```

Declare `hr-eracun-v1` on a document and `Calculate` + `Validate` will run the
eRačun normalization and rules. To import and export UBL, also add
`_ "github.com/invopop/gobl.hr.eracun/ubl"` and use `ubl.Import`/`ubl.Export`
from gobl.ubl, or `convert.Import`/`convert.Export`.

> **Note**: `hr-eracun-v1` is not yet in GOBL core's approved external-addon
> registry. It is added there once this module is released.

## Extensions

| Key | Description |
| --- | --- |
| `hr-eracun-process` | Business process (BT-23), written to the UBL `ProfileID`: `P1` to `P12`, or `P99:` followed by a label agreed with the buyer. |
| `hr-eracun-tax-category` | HR tax category of a tax combo (HR-TB-2): `PDV25`, `PDV13`, `PDV5`, `Z`, `K`, `G`, `AE`, `E`, `POVNAK`, `PP`, `PPMV`, `O` or `N`. Set from the tax key and rate, except for amounts collected for a third party (`POVNAK`, `PP`, `PPMV`) and any other non-taxable fee charged with taxable items (`N`), which are set on an exempt combo. |
| `hr-eracun-rejection-reason` | Kind of reason for rejecting an invoice (AR-BT-12): `N`, `U` or `O`, the same code the buyer reports to the Tax Administration. |

The `hr-fiskal-b2c-v1` addon sets these on the invoice tax:

| Key | Description |
| --- | --- |
| `hr-fiskal-premises` | Business premises label registered in ePorezna (`OznPosPr`): up to 20 letters and digits. Required. |
| `hr-fiskal-device` | Label of the device that issued the invoice (`OznNapUr`): up to 20 digits with no leading zero. Required. |
| `hr-fiskal-sequence-scope` | What the invoice numbers count within (`OznSlijed`): `P` per business premises (the default) or `N` per device. |
| `hr-fiskal-vat-system` | Whether the issuer is in the VAT system (`USustPdv`): `Y` (the default) or `N`. An issuer outside it charges no VAT. |
| `hr-fiskal-paper-number` | Number of an invoice from the bound paper invoice book, entered after the till broke down (`ParagonBrRac`): up to 100 characters. Such an invoice is sent late. |

The invoice code is the bare sequential number (`BrOznRac`): digits with no
leading zero. A printed invoice shows the number as
`BrOznRac/OznPosPr/OznNapUr`, such as `1001/POS1/1`.

The number can come from two places:

- **A till that numbers its own invoices** sends the code with the labels.
- **Invopop numbers the invoice.** The invoice arrives without a code, and the
  sequence step of the workflow gives it one. That step keeps one counter for
  each supplier and series, so the addon sets the series from the labels: the
  premises label for scope `P` (`POS1`), or `<premises>/<device>` for scope `N`
  (`POS1/1`). The step must use padding 0, as CIS refuses leading zeros.

The rules refuse a series that does not match the labels, so that a counter
never numbers two premises or two devices.

The `hr` regime's invoice tag `cash-basis` marks VAT accounted for when the
invoice is paid. The regime adds the legal VAT note, and the UBL export writes
`HRObracunPDVPoNaplati`.

## Operator

An eRačun names its operator, the person who issued it: a label in
`SellerContact/Name` (HR-BT-4) and their OIB in `SellerContact/ID` (HR-BT-5).
The OIB is also reported to the Tax Administration when the invoice is
fiscalized. Give the operator the `operator` key, a name and an `OIB` identity:

```yaml
supplier:
  people:
    - key: operator
      name:
        given: "Operater1"
      identities:
        - type: OIB
          code: "12345678903"
```

An invoice with no person with the key, more than one, or an operator
without a name or an OIB is rejected; no other person is taken in their place.
Article 8(4) of the Fiscalisation Act lets the supplier's own OIB stand in for
the operator's for a natural person's business, invoices issued through
self-service devices, and other cases where the operator's OIB can't be given.
In those cases, give the supplier's OIB as the operator's identity, as the
examples do. The operator is not written as the seller's contact (BG-6).
Correcting an invoice with GOBL leaves out the original operator, so the
corrective document needs its own.

## Issue time

An eRačun gives the time it was issued (HR-BR-2). An invoice with no
`issue_time` that is dated today in Croatia, or not dated at all, is given the
current date and time in Croatia (Europe/Zagreb). An invoice dated another day
is rejected until its `issue_time` is given. Correcting an invoice with GOBL
issues the corrective document now, so it doesn't keep the original's issue
time; if the correction is given an issue date other than today, set its
`issue_time` too.

## Validation

Rules register under the `HR-ERACUN` namespace, guarded so they apply only when
`hr-eracun-v1` is active. HR rules that GOBL cannot express stay with the
Tax Administration's schematron, which the tests run (see below).

## Invoice fiscalization request

`racun` sends CIS an invoice request, `RacunZahtjev`, with the schema's
Croatian element names. This is the request built from
`racun/testdata/examples/cash-prices-include-vat.json`, before it is signed,
with the meaning of each element. `racun/invoice.go` gives the Go name of
each field.

```xml
<!-- RacunZahtjev: invoice request. Sign adds an XML-DSig <Signature> that
     points at Id. -->
<RacunZahtjev xmlns="http://www.apis-it.hr/fin/2012/types/f73" Id="RacunZahtjev">
	<!-- Zaglavlje: message header -->
	<Zaglavlje>
		<!-- IdPoruke: message ID, a new UUID for each send -->
		<IdPoruke>00000000-0000-4000-8000-000000000000</IdPoruke>
		<!-- DatumVrijeme: when the message was sent -->
		<DatumVrijeme>06.11.2026T13:58:10</DatumVrijeme>
	</Zaglavlje>
	<!-- Racun: the invoice (račun) -->
	<Racun>
		<!-- Oib: the issuer's OIB, from supplier.tax_id -->
		<Oib>80432461483</Oib>
		<!-- USustPdv (u sustavu PDV-a): the issuer is registered for VAT -->
		<USustPdv>true</USustPdv>
		<!-- DatVrijeme: issue date and time, from issue_date and issue_time -->
		<DatVrijeme>06.11.2026T13:58:10</DatVrijeme>
		<!-- OznSlijed: what the number counts within, P (premises) or N (device) -->
		<OznSlijed>P</OznSlijed>
		<!-- BrRac: invoice number, printed as 1001/POS1/1 -->
		<BrRac>
			<!-- BrOznRac: sequential number, from the invoice code -->
			<BrOznRac>1001</BrOznRac>
			<!-- OznPosPr: business premises label, registered with the Tax
			     Administration -->
			<OznPosPr>POS1</OznPosPr>
			<!-- OznNapUr: device (till) label -->
			<OznNapUr>1</OznNapUr>
		</BrRac>
		<!-- Pdv: VAT, one Porez for each rate -->
		<Pdv>
			<Porez>
				<!-- Stopa: rate, in percent ("general" in GOBL) -->
				<Stopa>25.00</Stopa>
				<!-- Osnovica: taxable base. The prices include VAT, so
				     2 × 2.50 = 5.00 is 4.00 base and 1.00 VAT. -->
				<Osnovica>4.00</Osnovica>
				<!-- Iznos: VAT amount -->
				<Iznos>1.00</Iznos>
			</Porez>
			<Porez>
				<!-- "reduced" in GOBL: 5.65 is 5.00 base and 0.65 VAT -->
				<Stopa>13.00</Stopa>
				<Osnovica>5.00</Osnovica>
				<Iznos>0.65</Iznos>
			</Porez>
		</Pdv>
		<!-- IznosUkupno: total, VAT included -->
		<IznosUkupno>10.65</IznosUkupno>
		<!-- NacinPlac (način plaćanja): payment method, see the table below -->
		<NacinPlac>G</NacinPlac>
		<!-- OibOper: the operator's OIB. Here the issuer's own OIB stands in. -->
		<OibOper>80432461483</OibOper>
		<!-- ZastKod (zaštitni kod): the ZKI, the MD5 of the RSA-SHA256 signature
		     of Oib, DatVrijeme, the three BrRac parts and IznosUkupno -->
		<ZastKod>29f16dcad4dc85441c183d3826c22985</ZastKod>
		<!-- NakDost (naknadna dostava): late delivery, true for an invoice
		     issued while CIS could not be reached and sent afterwards -->
		<NakDost>false</NakDost>
	</Racun>
</RacunZahtjev>
```

| `NacinPlac` | Payment method |
| --- | --- |
| `G` | Cash (gotovina) |
| `K` | Card (kartice) |
| `T` | Bank transfer (transakcijski račun) |
| `O` | Other, or more than one method |

The schema also has elements this example leaves out: consumption tax
(`Pnp`), other taxes (`OstaliPor`), the VAT-exempt amount (`IznosOslobPdv`),
the margin-scheme amount (`IznosMarza`), the amount not subject to tax
(`IznosNePodlOpor`), fees (`Naknade`), the number of an invoice from the
bound paper invoice book, issued while the device was down (`ParagonBrRac`),
a field the Tax Administration reserves (`SpecNamj`) and the buyer's OIB
(`OibPrimateljaRacuna`), given only when the buyer is a business.

CIS answers with `RacunOdgovor`. When it accepts the invoice, the answer holds
the JIR (`Jir`, the unique invoice identifier); when it refuses it, the answer
holds `Greske` (errors), each with a code and a message.

## Development

### Dependencies not yet released

> **Before tagging or merging**, remove the `replace` block in `go.mod`. It
> pins gobl and gobl.ubl to unreleased branch commits, through their
> pseudo-versions, so that CI can build the module until those branches are
> released.

| Module | Waits for | Replaced by | Pin |
| --- | --- | --- | --- |
| `github.com/invopop/gobl` | the `hr` regime (branch `eu3-18-hr-regime`, a rewrite on the Slovakia template, which [invopop/gobl#724](https://github.com/invopop/gobl/pull/724) now carries) | branch commit `70ccde1c` | the gobl release that includes it, or a `main` pseudo-version after merge |
| `github.com/invopop/gobl.ubl` | the `ExtensionContent` fix that registers extension data types ([invopop/gobl.ubl#183](https://github.com/invopop/gobl.ubl/pull/183), branch `extension-content-any`), then the generic fields the HR format uses (`SellerContact`, tax category `Name`), restacked on that fix ([invopop/gobl.ubl#179](https://github.com/invopop/gobl.ubl/pull/179), branch `eu3-18-hr-fields`) | branch commit `e1af3b2`, the head of #179 (with #183 merged in) | the gobl.ubl release that includes both |

To follow a newer commit of either branch, run for example
`go mod edit -replace=github.com/invopop/gobl=github.com/invopop/gobl@<commit>`
then `go mod tidy`. To work against local checkouts instead, point the
`replace` at their paths, and don't commit that.

Add the `release.yaml` workflow of the other GOBL modules at the same time:
it tags every push to `main`, and a tag made while the `replace` block is in
place cannot be built by any consumer.

### Tests

```sh
go test ./...
```

### Invoice fiscalization (`racun`)

The tests convert the examples in `racun/testdata/examples` and compare them
with `racun/testdata/out` (`-update` writes them again; the ZKI uses the
fixed test key `racun/testdata/test-key.pem`).

Every call to CIS is replayed from a cassette in `racun/testdata/cistest/`:
CIS test's real, signed answer (`<name>-answer.xml`) and what the replay
needs (`<name>.json`: the message ID, when it was sent, the HTTP status and
the JIR or error code). A normal run fiscalizes each scenario (an accepted
invoice, a refused one and each example) through the whole client, offline:
it rebuilds the request for the recorded time and message ID, and checks
CIS's signature on the recorded answer. Only CIS can sign an answer, so the
cassettes come from CIS test. To record them again, run with `-record` and
a FINA demo certificate (keep it in `.certs/`, which git ignores):

```sh
export CIS_TEST_CERT=$PWD/.certs/demo.p12
read -rs CIS_TEST_PASSWORD && export CIS_TEST_PASSWORD
go test ./racun/ -run 'TestCISTest' -record -count=1 -v
unset CIS_TEST_PASSWORD
```

`-xsd` checks signed requests and the expected invoices against
`FiskalizacijaSchema.xsd` v1.10 with xmllint in Docker. CI runs it.

### Examples

`examples/` holds GOBL documents (an invoice, a credit note and a rejection),
with their expected JSON envelopes under `examples/out/`. They are verified
via GOBL's shared `pkg/examples` helpers. Regenerate the golden output after
intentional changes with:

```sh
go test . -run TestExamples -update
```

### UBL fixtures

`ubl/testdata/hr/` holds the 20 official HR CIUS examples of the Tax
Administration, patched so that they pass the HR rules: a valid OIB in place
of the placeholder, 2026 dates and a KPD on the leasing line.
[ubl/testdata/hr/README.md](ubl/testdata/hr/README.md) lists the changes.
`ubl/testdata/hr-ar/` holds the rejection example of the application response
specification and an acceptance and a query built from it, with placeholder
OIBs
([README](ubl/testdata/hr-ar/README.md)). They are official sample values,
not customer data.

Each fixture is imported into GOBL and exported back to UBL; both results are
compared with the files under `out/`. Regenerate them after intentional
changes, and review the diff:

```sh
go test ./ubl/ -update
```

### Schematron

Generated invoices and credit notes are validated with
[phorm](https://github.com/phax/phorm) 2.2.11 or newer, the first release with
the HR rules 1.0.4. Each rule set runs the UBL 2.1 schema, the CEN EN 16931
schematron and the HR CIUS extension schematron. Run phorm locally, bound to
the loopback interface only:

```sh
docker run -d --rm --name phorm -p 127.0.0.1:8080:8080 phelger/phorm:2.2.11
PHORM_URL=http://127.0.0.1:8080 go test ./ubl/ -validate
```

The `-validate` flag is declared in the `ubl` package only, so scope the run
to `./ubl/`. Warnings are not failures: every HR document gets EN 16931
warnings for elements the HR CIUS requires, such as `UBL-CR-006` (IssueTime),
`UBL-CR-200` (SellerContact) and `UBL-CR-597` (tax category name on lines).

### Application response schema

phorm has no rule set for the HR application response, and its specification
publishes no schema of its own: the message is a plain UBL 2.1
ApplicationResponse. The test checks the values the specification fixes, and
with `-xsd` also validates the export against the OASIS UBL 2.1 schema in
`ubl/testdata/schema/` with xmllint, run in an `alpine` container. It needs
Docker:

```sh
go test ./ubl/ -run TestApplicationResponse -xsd
```

CI runs all three: the tests with coverage, the schematron against a phorm
service container, and the schema check.

## Sources

- [Porezna uprava - eRačun](https://porezna.gov.hr/fiskalizacija/bezgotovinski-racuni/eracun)
- [Specifikacija osnovne uporabe eRačuna s proširenjima](https://porezna.gov.hr/fiskalizacija/api/dokumenti/196) (HR CIUS 2025 and the national extension)
- [Tehnička specifikacija: Fiskalizacija eRačuna i eIzvještavanje v1.15](https://porezna.gov.hr/fiskalizacija/api/dokumenti/186) (CIS fiscalization and eReporting, including the rejection reason codes)
- [HR UBL schematron of 13.03.2026](https://porezna.gov.hr/fiskalizacija/api/dokumenti/197) (rules 1.0.4)
- [Aplikacijski odgovor v1.1](https://porezna.gov.hr/fiskalizacija/api/dokumenti/206) (application response)
- Fiskalizacija računa u krajnjoj potrošnji, tehnička specifikacija v2.7, and `FiskalizacijaSchema.xsd` v1.10 (invoice fiscalization)

## License

Apache 2.0 — see [LICENSE](./LICENSE).
