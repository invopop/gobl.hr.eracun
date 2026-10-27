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
- the application response a buyer sends to reject an eRačun
  (`urn:mfin.gov.hr:AplikacijskiOdgovor-2025:1.0`), mapped to `bill.Status`.

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
| `hr-eracun-tax-category` | HR tax category of a tax combo (HR-TB-2): `PDV25`, `PDV13`, `PDV5`, `Z`, `K`, `G`, `AE`, `E`, `POVNAK`, `PP`, `PPMV`, `O` or `N`. Set from the tax key and rate, except for amounts collected for a third party (`POVNAK`, `PP`, `PPMV`) and any other non-taxable fee charged with taxable items (`N`), which are set on an exempt combo. |
| `hr-eracun-rejection-reason` | Kind of reason for rejecting an invoice (AR-BT-12): `N`, `U` or `O`, the same code the buyer reports to the Tax Administration. |

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
| `github.com/invopop/gobl` | the `hr` regime (branch `eu3-18-hr-regime`, a rewrite of [invopop/gobl#724](https://github.com/invopop/gobl/pull/724) on the Slovakia template; PR pending) | branch commit `70ccde1c` | the gobl release that includes it, or a `main` pseudo-version after merge |
| `github.com/invopop/gobl.ubl` | the generic fields the HR format uses (`SellerContact`, tax category `Name`, raw `ExtensionContent`) (branch `eu3-18-hr-fields`) | branch commit `a45bc23` | the gobl.ubl release that includes them |

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
specification, with placeholder OIBs
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
