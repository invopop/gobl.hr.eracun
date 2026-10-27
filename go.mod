module github.com/invopop/gobl.hr.eracun

go 1.25.0

require (
	github.com/invopop/gobl v0.510.0
	github.com/invopop/gobl.ubl v0.94.0
	github.com/invopop/phorm v0.1.5
	github.com/invopop/xmlctx v0.14.0
	github.com/stretchr/testify v1.11.1
)

require (
	cloud.google.com/go v0.118.0 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beevik/etree v1.6.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/invopop/validation v0.8.0 // indirect
	github.com/invopop/xmldsig v0.14.0 // indirect
	github.com/invopop/yaml v0.3.1 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russellhaering/goxmldsig v1.6.0 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	software.sslmate.com/src/go-pkcs12 v0.7.0 // indirect
)

// LOCAL DEVELOPMENT ONLY: remove before tagging or merging. These point at
// local checkouts of the unreleased hr regime (gobl) and the generic HR fields
// (gobl.ubl), so the module does not build anywhere else, CI included. Pin
// both to released versions (or main pseudo-versions) once they are merged;
// the README lists what each pin waits for.
replace (
	github.com/invopop/gobl => github.com/invopop/gobl v0.510.1-0.20261027164024-70ccde1c0e68
	github.com/invopop/gobl.ubl => github.com/invopop/gobl.ubl v0.97.1-0.20261026145643-a45bc237723a
)
