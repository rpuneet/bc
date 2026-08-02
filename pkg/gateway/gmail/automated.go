package gmailgw

import "strings"

// Machine-generated mail — GitHub notifications, newsletters, bank alerts,
// bounces — arrives on the same poll as human mail, but replying to it is
// pointless and waking every subscribed agent for it is expensive. The
// adapter classifies each message so the notify layer can ingest it into the
// channel feed without prompting agents.
//
// Classification prefers standard headers over address-name guesses: a
// sender's local part is a heuristic, whereas Auto-Submitted and Precedence
// are the sender declaring its own intent.

// automatedHeaders are the headers fetched purely for classification, on top
// of the display headers parseMessage needs.
var automatedHeaders = []string{
	"Auto-Submitted",
	"Precedence",
	"List-Unsubscribe",
	"X-Auto-Response-Suppress",
}

// noReplyLocalParts are address local parts that never belong to a human
// mailbox. Deliberately conservative: role addresses like support@, info@ and
// alerts@ are frequently human-staffed, so they are not listed here.
var noReplyLocalParts = []string{
	"noreply",
	"no-reply",
	"no_reply",
	"no.reply",
	"donotreply",
	"do-not-reply",
	"do_not_reply",
	"notification",
	"notifications",
	"mailer-daemon",
	"postmaster",
	"bounce",
	"bounces",
	"autoreply",
	"auto-reply",
}

// bulkPrecedence are Precedence values that mark mail as bulk-sent rather
// than a person writing to you. "list" is excluded on purpose — genuine team
// discussion lists (Google Groups and similar) set it, and those are
// conversations agents should still see.
var bulkPrecedence = []string{"bulk", "junk"}

// classifyAutomated reports whether a message is machine-generated, plus a
// short human-readable reason for logs and the stored payload. The reason
// names the signal that fired so an operator can tell a header declaration
// from a local-part guess when a message unexpectedly skips agent delivery.
func classifyAutomated(from string, headers map[string]string) (bool, string) {
	// RFC 3834: any value other than "no" means the message was generated
	// automatically. GitHub sets "auto-generated" on notification mail.
	if v := headerValue(headers, "Auto-Submitted"); v != "" && v != "no" {
		return true, "auto-submitted: " + v
	}

	// RFC 3834 §5 suppression hints are only ever set by automated systems.
	if headerValue(headers, "X-Auto-Response-Suppress") != "" {
		return true, "x-auto-response-suppress"
	}

	if v := headerValue(headers, "Precedence"); v != "" {
		for _, p := range bulkPrecedence {
			if v == p {
				return true, "precedence: " + p
			}
		}
	}

	// Anything offering a machine-readable unsubscribe is a mailing to many
	// recipients, not a message written to this mailbox.
	if headerValue(headers, "List-Unsubscribe") != "" {
		return true, "list-unsubscribe"
	}

	if local := localPart(from); local != "" {
		for _, np := range noReplyLocalParts {
			// Exact match, or a prefixed/suffixed variant such as
			// "github-noreply" or "noreply-bounces".
			if local == np || strings.HasPrefix(local, np+"-") || strings.HasSuffix(local, "-"+np) {
				return true, "no-reply sender: " + local
			}
		}
	}

	return false, ""
}

// headerValue looks up a header case-insensitively and returns it lowercased
// and trimmed, ready for comparison.
func headerValue(headers map[string]string, name string) string {
	if headers == nil {
		return ""
	}
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

// localPart extracts the lowercased portion before "@" of a From header,
// tolerating a display name around the address.
func localPart(from string) string {
	addr := extractEmail(from)
	local, _, found := strings.Cut(addr, "@")
	if !found {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(local))
}
