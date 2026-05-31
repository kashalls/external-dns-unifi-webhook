package unifi

// Shared constants for the unifi package tests. These exist to satisfy the
// goconst linter, which flags string literals repeated across the test files.
const (
	// Hosts and base URLs.
	testHost       = "https://unifi.local"
	testHostPort   = "https://192.168.1.1:8443"
	testURLExample = "https://example.com"
	testSite       = "default"
	testSiteUUID   = "11111111-2222-3333-4444-555555555555"
	testRecordID   = "507f1f77bcf86cd799439011"

	// DNS record field values.
	testIPv4A   = "192.168.1.1"
	testIPv4B   = "192.168.1.2"
	testSRVName = "_service._tcp.example.com"
	testAlias   = "alias.example.com"
	testCNAMEID = "cname1"
	testErrCode = "ERROR"

	// HTTP headers.
	headerAccept      = "Accept"
	headerContentType = "Content-Type"

	// Error message fragments.
	testMsgInternalServer = "Internal server error"
	testMsgForbidden      = "forbidden"
	testMsgRecordNotFound = "Record not found"
	testMsgUnknownError   = "Unknown error"
	testMsgGenericError   = "generic error"
	testAuthFailedLogin   = "authentication failed during login"
	testAPIErrPost        = "API error during POST"

	// DataError data types.
	testDataDNSRecord = "DNS record"
	testDataAPIResp   = "API response"
	testDataSRVTarget = "SRV record target"

	// URLs used in error and FormatURL tests.
	testURLLoginExample  = "https://unifi.example.com/api/login"
	testURLInvalid       = "https://invalid.local/api"
	testURLSpecialChars  = "https://unifi.local/api?param=value&key=секрет"
	testURLMissingRecord = "https://unifi.local/api/site/default/static-dns/missing"
	testURLLoginInternal = "https://unifi.local/api/auth/login"

	// Subtest names.
	testNameAuthError = "AuthError"

	// FormatURL path fragments.
	testPathHostParam = "%s/api/%s"
	testPathHost      = "%s/api"
	testPathAPISlash  = "/api/"
	testPathSingle    = "%s"
	testPathTriple    = "%s%s%s"

	// DNS record IDs, names and values.
	testDNSID1    = "record1"
	testDNSID2    = "record2"
	testTarget    = "target.example.com"
	testMultiName = "multi.example.com"

	// Client credentials and content types.
	testKeyA            = "test-key"
	testKeyB            = "test-api-key"
	contentTypeJSON     = "application/json"
	contentTypeJSONUTF8 = "application/json; charset=utf-8"

	// Operations, statuses and messages.
	testOpMarshal      = "marshal"
	testMsgInvalidAuth = "invalid credentials"
	testMsgInvalid     = "invalid"
	testMsgNoResponse  = "no response"
	testStatus401      = "status 401"
	testStatus0        = "status 0"
	testDataRespBody   = "response body"

	// Subtest names.
	testNameNilError = "nil error"

	// URL containing credentials, used to exercise FormatURL.
	testURLCreds = "https://user:pass@example.com" //nolint:gosec // G101: test fixture URL with embedded credentials
)
