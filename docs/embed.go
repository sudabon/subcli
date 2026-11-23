package docs

import _ "embed"

// ComplianceMarkdown holds the embedded copy of docs/COMPLIANCE.md so binary-only
// installations can still verify the terms confirmation date.
//
//go:embed COMPLIANCE.md
var ComplianceMarkdown string
