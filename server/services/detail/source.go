package detail

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/utils/context"
)

// GetEmailSource reconstructs an RFC 822-like source view from the stored
// message. The parser does not keep the original wire bytes, so this is a
// faithful reconstruction of the headers and body that iMail stored.
func GetEmailSource(ctx *context.Context, id int) (string, error) {
	message, err := GetEmailDetail(ctx, id, false)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message.RawMIME) != "" {
		return message.RawMIME, nil
	}

	formatUsers := func(raw string) string {
		if strings.TrimSpace(raw) == "" {
			return ""
		}
		var users []parsemail.User
		if err := json.Unmarshal([]byte(raw), &users); err != nil {
			return ""
		}
		parts := make([]string, 0, len(users))
		for _, user := range users {
			if user.EmailAddress == "" {
				continue
			}
			if user.Name != "" {
				parts = append(parts, fmt.Sprintf("%s <%s>", cleanHeader(user.Name), user.EmailAddress))
			} else {
				parts = append(parts, fmt.Sprintf("<%s>", user.EmailAddress))
			}
		}
		return strings.Join(parts, ", ")
	}

	contentType := "text/plain; charset=UTF-8"
	body := message.Text.String
	if message.Html.String != "" {
		contentType = "text/html; charset=UTF-8"
		body = message.Html.String
	}

	var source strings.Builder
	writeHeader := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&source, "%s: %s\r\n", name, cleanHeader(value))
		}
	}
	writeHeader("From", formatFrom(message.FromName, message.FromAddress))
	writeHeader("To", formatUsers(message.To))
	writeHeader("Cc", formatUsers(message.Cc))
	writeHeader("Subject", message.Subject)
	writeHeader("Date", message.SendDate.UTC().Format(time.RFC1123Z))
	if message.MsgID != "" {
		writeHeader("Message-ID", "<"+message.MsgID+">")
	}
	writeHeader("MIME-Version", "1.0")
	writeHeader("Content-Type", contentType)
	source.WriteString("\r\n")
	source.WriteString(body)
	return source.String(), nil
}

func formatFrom(name, address string) string {
	if address == "" {
		return ""
	}
	if name != "" {
		return fmt.Sprintf("%s <%s>", cleanHeader(name), address)
	}
	return fmt.Sprintf("<%s>", address)
}

func cleanHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
