# UBL 2.1 schema

The OASIS UBL 2.1 OS schema files that the ApplicationResponse needs,
unchanged from https://docs.oasis-open.org/ubl/os-UBL-2.1/xsd/. The `common/`
files are the same as gobl.ubl's `test/data/schema/common/`.

The HR application response specification (AplikacijskiOdgovor v1.1)
publishes no schema of its own, so `go test ./ubl/ -xsd` checks the generated
application response against `maindoc/UBL-ApplicationResponse-2.1.xsd`.
