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

The Croatian tax regime itself (`regimes/hr`) lives in GOBL core. This module
only carries the eRačun addon and format, so that only projects handling
Croatian eRačuni take on their weight.

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
| `hr-eracun-tax-category` | HR tax category of a tax combo (HR-TB-2): `PDV25`, `PDV13`, `PDV5`, `Z`, `K`, `G`, `AE`, `E`, `POVNAK`, `PP`, `PPMV`, `O` or `N`. Set from the tax key and rate, except for amounts collected for a third party (`POVNAK`, `PP`, `PPMV`) and any other non-taxable fee charged with taxable items (`N`), which are set on an exempt combo. An issuer charges the consumption tax with the regime's `PP` tax instead (see [Consumption tax (PP)](#consumption-tax-pp)); the label `PP` is how a received eRačun carries it. |
| `hr-eracun-rejection-reason` | Kind of reason for rejecting an invoice (AR-BT-12): `N`, `U` or `O`, the same code the buyer reports to the Tax Administration. |

The `hr` regime's invoice tag `cash-basis` marks VAT accounted for when the
invoice is paid. The regime adds the legal VAT note, and the UBL export writes
`HRObracunPDVPoNaplati`.

## Exemption reasons

Every exempt (E) or not taxable (O) line and document level charge needs an
exemption reason (HR-BR-13, HR-BR-36). The UBL export writes the text of the
invoice's VAT note for its category, except for an amount left out of the VAT
base (`POVNAK`, `PP`, `PPMV` or `N`). That amount gets the name of its label:
"Povratna naknada", "Porez na potrošnju", "Posebni porez na motorna vozila" or
"Neoporeziva naknada", as the official examples write it on a POVNAK, PP or
PPMV charge. So the `exempt` VAT note gives the legal ground of the exempt
supplies, and it is also the one reason of the EN 16931 VAT breakdown, where
all E amounts share one entry (BR-E-01).

To give a line or charge a reason of its own, such as "Turistička pristojba"
for a tourist tax, set the meta `hr-eracun-tax-exemption-reason` on the line's
item, or on the charge or discount. The import sets it when a received reason
differs from the one the export would write.

## Consumption tax (PP)

The consumption tax (porez na potrošnju) on drinks served in catering is the
`hr` regime's tax category `PP`. Charge it on each line it applies to, beside
the line's VAT, at the rate the municipality or city sets, up to 3%:

```yaml
lines:
  - quantity: "3"
    item:
      name: "Pivo 0,5 l"
      price: "4.00"
    taxes:
      - cat: VAT
        rate: general
      - cat: PP
        percent: "3%"
```

The HR CIUS has no tax of that kind: it carries PP as an amount left out of
the VAT base. The UBL export writes it the way the official example
"eRacun-PDV-NEOP-PP-trosak" does:

- one document level charge per PP rate, exempt from VAT (`E`, named `HR:PP`),
  with the PP of the lines at that rate as its amount, their net amounts as
  its base and the rate as its multiplier;
- an exempt entry for those charges in the VAT breakdown, and an entry out of
  the VAT scope (`O`) under the `LOC` scheme in the HR breakdown;
- the VAT alone as the VAT total (BT-110), and the PP charges in the tax
  exclusive amount (BT-109) and the charges total (BT-108).

Each line with PP also gets the item property `HR:PP` with its percent, so
the buyer can tell which lines a charge covers. The HR CIUS has no element for
that, so the property is this module's own convention, and a line with PP
must not have an attribute of that name.

The export can't write PP that has no base in the lines, so PP on a document
level discount or charge is rejected: discount or charge the lines instead. A
line with PP needs a VAT combo too, and a supplier outside the VAT system
can't charge PP yet. Nor can an invoice with amounts not subject to VAT (O):
EN 16931 allows no exempt amount beside them (BR-O-11). An invoice with the PP
tax on its lines can't also have a line, charge or discount with the label
`hr-eracun-tax-category: PP`, the form received invoices carry PP in.

Limits:

- Importing an eRačun reads PP as that exempt charge, with the `HR:PP` item
  attributes, not as the PP tax on the lines.
- A price that includes VAT (`prices_include: VAT`) doesn't include PP, which
  is added on top of it.
- A document level discount without PP lowers the VAT base, but not the PP
  base.

## Supplier

A supplier in the VAT system gives its Croatian VAT number in `tax_id`, and
the export writes it as the seller's VAT identifier (BT-31), `HR<OIB>`. A
supplier outside the VAT system, such as a small taxpayer, has no VAT number:
its `tax_id` has only the country, which keeps the regime, and its OIB goes in
an `OIB` identity:

```yaml
supplier:
  tax_id:
    country: "HR"
  identities:
    - type: OIB
      code: "12345678903"
```

The export writes that OIB as the seller's legal registration (BT-30), as its
tax registration (BT-32) with the scheme `FRE`, and as its eRačun address
`9934:<OIB>`, as in the official example "eRacun-Nije u sustavu PDV". An `OIB`
identity with no scope gets the legal scope, unless another identity already
has it. The invoice must not charge VAT, and its S, Z, E and AE taxes need the
`OIB` identity. Importing an eRačun with a BT-32 and no BT-31 gives the same
supplier. The `hr-fiskal-b2c-v1` addon takes the same party, and gobl.fr.ctc
models a French supplier the same way, with a SIREN identity.

## eRačun addresses and buyers

Both parties need an eRačun address, written as the UBL `EndpointID` (BT-34,
BT-49). Every participant is registered in AMS under its OIB, as
`iso6523-actorid-upis::9934:<OIB>`, and may also be registered under other
ISO 6523 identifiers, such as a GLN (`0088`). The addon gives a party with an
OIB, from its Croatian VAT number or an `OIB` identity, and no address the
address `9934:<OIB>`. An address given with the scheme `9934` must be a valid
OIB, the party's own when it has one. Other schemes are accepted as given.

An eRačun goes only to a taxpayer established in Croatia (Fiscalisation Act,
Art. 2(24) and 38), so a customer with a foreign tax ID is refused, even though
the HR schematron accepts foreign parties. The customer also needs its OIB,
which CIS records for every eRačun: a Croatian VAT number, an `OIB` identity or
a `9934:<OIB>` address. An intra-community supply, or any invoice to a foreign
buyer, is issued on paper or in another form the tax rules accept (Tax
Administration FAQ, eRačun fiscalization, question 8). An export (`G`) to a
Croatian buyer is still an eRačun.

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

## Rounding

An eRačun's VAT total must be the sum of its VAT breakdown amounts, each one
its base times its rate, rounded to the cent (BR-CO-14, BR-CO-17). GOBL's
default `precise` rounding rounds the VAT once, on the sum of the unrounded
amounts, which can leave the total a cent off the breakdown on an invoice with
more than one VAT rate. An invoice with no `tax.rounding` is given `currency`,
which rounds each amount before adding them up, and an invoice that sets any
other rule is rejected.

## KPD classification

A line item gives its KPD 2025 code (Klasifikacija proizvoda po
djelatnostima) as an identity with the label `CG`, which the export writes to
the item's classification code (BT-158) with `listID="CG"`:

```yaml
item:
  name: "Proizvod"
  identities:
    - label: CG
      code: "62.20.20"
```

Every line needs one, except on advance invoices and credit notes (HR-BR-25).
A `CG` code on any document type must be written as the official code list
writes it, three pairs of digits joined by dots, or the invoice is rejected.
Only that form is checked: a well-formed code that is not in the list passes
GOBL, and the Tax Administration's schematron refuses it (HR-BR-CL-2).

## Validation

Rules register under the `HR-ERACUN` namespace, guarded so they apply only when
`hr-eracun-v1` is active. HR rules that GOBL cannot express stay with the
Tax Administration's schematron, which the tests run (see below).

## Development

### Dependencies not yet released

> **Before tagging or merging**, remove the `replace` block in `go.mod`. It
> pins gobl and gobl.ubl to unreleased branch commits, through their
> pseudo-versions, so that CI can build the module until those branches are
> released.

| Module | Waits for | Replaced by | Pin |
| --- | --- | --- | --- |
| `github.com/invopop/gobl` | the `hr` regime (branch `eu3-18-hr-regime`, a rewrite on the Slovakia template, which [invopop/gobl#724](https://github.com/invopop/gobl/pull/724) now carries) | branch commit `8558e34a`, which adds the `PP` consumption tax category | the gobl release that includes it, or a `main` pseudo-version after merge |
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

## License

Apache 2.0 — see [LICENSE](./LICENSE).
