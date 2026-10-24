package types

import "strings"

// TOKEN: customers never see database internals. Errors from the database
// layer (for example during a failover) reach clients as a generic,
// retryable message; the full error stays in the server log.

const tokenTemporaryFailureMessage = "The service is temporarily unavailable. Please retry in a few seconds."

var tokenDatabaseErrorMarkers = []string{"SQLSTATE", "sql: ", "driver: bad connection", "pq: ", "database is locked", "connection reset by peer"}

func tokenCustomerSafeMessage(code ErrorCode, msg string) string {
	if code == ErrorCodeQueryDataError || code == ErrorCodeUpdateDataError {
		return tokenTemporaryFailureMessage
	}
	for _, marker := range tokenDatabaseErrorMarkers {
		if strings.Contains(msg, marker) {
			return tokenTemporaryFailureMessage
		}
	}
	return msg
}
