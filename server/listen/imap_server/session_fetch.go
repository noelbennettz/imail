package imap_server

import (
	"bytes"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/services/list"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/spf13/cast"
)

// userToAddress converts a parsemail.User into an imap.Address
func userToAddress(u *parsemail.User) imap.Address {
	if u == nil {
		return imap.Address{}
	}
	// Split the email address into its local and domain parts
	mailbox := u.EmailAddress
	host := ""
	if at := strings.LastIndex(u.EmailAddress, "@"); at >= 0 {
		mailbox = u.EmailAddress[:at]
		host = u.EmailAddress[at+1:]
	}
	return imap.Address{
		Name:    u.Name,
		Mailbox: mailbox,
		Host:    host,
	}
}

// usersToAddresses converts a list of users into a list of addresses
func usersToAddresses(users []*parsemail.User) []imap.Address {
	if len(users) == 0 {
		return nil
	}
	addrs := make([]imap.Address, 0, len(users))
	for _, u := range users {
		if u != nil && u.EmailAddress != "" {
			addrs = append(addrs, userToAddress(u))
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	return addrs
}

// buildEnvelope builds the IMAP ENVELOPE structure
func buildEnvelope(email *response.EmailResponseData, traEmail *parsemail.Email) *imap.Envelope {
	// From address
	var from []imap.Address
	if email.FromAddress != "" {
		mailbox := email.FromAddress
		host := ""
		if at := strings.LastIndex(email.FromAddress, "@"); at >= 0 {
			mailbox = email.FromAddress[:at]
			host = email.FromAddress[at+1:]
		}
		from = []imap.Address{{
			Name:    email.FromName,
			Mailbox: mailbox,
			Host:    host,
		}}
	}

	// Sender (falls back to from when there is no separate sender)
	var sender []imap.Address
	if traEmail.Sender != nil && traEmail.Sender.EmailAddress != "" {
		sender = []imap.Address{userToAddress(traEmail.Sender)}
	} else {
		sender = from
	}

	// Reply-To
	var replyTo []imap.Address
	if len(traEmail.ReplyTo) > 0 {
		replyTo = usersToAddresses(traEmail.ReplyTo)
	} else {
		replyTo = from
	}

	// Message-ID: use the persisted value so IMAP clients see a stable ID on every sync.
	var messageID string
	if email.MsgID != "" {
		messageID = fmt.Sprintf("<%s>", email.MsgID)
	} else {
		messageID = fmt.Sprintf("<%d@%s>", email.Id, config.Instance.Domain)
	}

	return &imap.Envelope{
		Date:      email.CreateTime,
		Subject:   email.Subject,
		From:      from,
		Sender:    sender,
		ReplyTo:   replyTo,
		To:        usersToAddresses(traEmail.To),
		Cc:        usersToAddresses(traEmail.Cc),
		Bcc:       usersToAddresses(traEmail.Bcc),
		MessageID: messageID,
		// InReplyTo and References are not supported yet
	}
}

func (s *serverSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	switch set := numSet.(type) {
	case imap.SeqSet:
		for _, seq := range set {
			emailList := list.GetEmailListByGroup(s.ctx, s.currentMailbox, list.ImapListReq{
				Star: cast.ToInt(seq.Start),
				End:  cast.ToInt(seq.Stop),
			}, false)
			write(s.ctx, w, emailList, options)
		}

	case imap.UIDSet:
		for _, uid := range set {
			emailList := list.GetEmailListByGroup(s.ctx, s.currentMailbox, list.ImapListReq{
				Star: cast.ToInt(uint32(uid.Start)),
				End:  cast.ToInt(uint32(uid.Stop)),
			}, true)
			write(s.ctx, w, emailList, options)
		}
	}
	return nil
}

// Plain text text/plain
func bsTextPlain(size uint32, numLines int64) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:     "text",
		Subtype:  "plain",
		Params:   map[string]string{"charset": "utf-8"},
		Encoding: "base64",
		Size:     size, // in bytes
		Text:     &imap.BodyStructureText{NumLines: numLines},
		Extended: &imap.BodyStructureSinglePartExt{},
	}
}

// HTML text/html
func bsTextHTML(size uint32, numLines int64) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:     "text",
		Subtype:  "html",
		Params:   map[string]string{"charset": "utf-8"},
		Encoding: "base64",
		Size:     size,
		Text:     &imap.BodyStructureText{NumLines: numLines},
		Extended: &imap.BodyStructureSinglePartExt{},
	}
}

// Generic attachment (pass the MIME type, e.g. "application/pdf")
func bsAttachment(filename, mime string, size uint32, encoding string) *imap.BodyStructureSinglePart {
	mt, st := "application", "octet-stream"
	if slash := strings.IndexByte(mime, '/'); slash > 0 {
		mt, st = mime[:slash], mime[slash+1:]
	}
	return &imap.BodyStructureSinglePart{
		Type:     mt,
		Subtype:  st,
		Params:   map[string]string{"name": filename}, // alternative name
		ID:       "",                                  // may hold the Content-ID
		Encoding: encoding,                            // usually "base64"
		Size:     size,
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value:  "attachment",
				Params: map[string]string{"filename": filename}, // clients prefer this one
			},
		},
	}
}

// multipart/alternative: text + html
// Note: BodyStructureMultiPart must have at least one child, otherwise go-imap panics
func bsAlternative(text, html *imap.BodyStructureSinglePart) *imap.BodyStructureMultiPart {
	var children []imap.BodyStructure

	if text != nil {
		children = append(children, text)
	}
	if html != nil {
		children = append(children, html)
	}

	// If there is no content at all, create an empty text/plain placeholder
	// This avoids the go-imap panic: "imap.BodyStructureMultiPart must have at least one child"
	if len(children) == 0 {
		children = append(children, bsTextPlain(0, 0))
	}

	return &imap.BodyStructureMultiPart{
		Subtype:  "alternative",
		Children: children,
		Extended: &imap.BodyStructureMultiPartExt{},
	}
}

// multipart/mixed：{ alternative(text+html), attachments... }
func bsMixedWithAttachments(alt *imap.BodyStructureMultiPart, extend bool, atts ...imap.BodyStructure) *imap.BodyStructureMultiPart {
	children := []imap.BodyStructure{alt}
	children = append(children, atts...)
	var ext *imap.BodyStructureMultiPartExt
	if extend {
		ext = &imap.BodyStructureMultiPartExt{}
	}
	return &imap.BodyStructureMultiPart{
		Subtype:  "mixed",
		Children: children,
		Extended: ext,
	}
}

func write(ctx *context.Context, w *imapserver.FetchWriter, emailList []*response.EmailResponseData, options *imap.FetchOptions) {
	for _, email := range emailList {
		writer := w.CreateMessage(cast.ToUint32(email.SerialNumber))

		traEmail := parsemail.NewEmailFromModel(email.Email)

		if options.UID {
			writer.WriteUID(imap.UID(email.UeId))
		}
		if options.Envelope {
			env := buildEnvelope(email, traEmail)
			writer.WriteEnvelope(env)
		}
		if options.BodyStructure != nil {
			var html, text *imap.BodyStructureSinglePart
			if len(traEmail.HTML) > 0 {
				html = bsTextHTML(uint32(len(traEmail.HTML)), int64(bytes.Count(traEmail.HTML, []byte("\n"))+1))
			}

			if len(traEmail.Text) > 0 {
				text = bsTextPlain(uint32(len(traEmail.Text)), int64(bytes.Count(traEmail.Text, []byte("\n"))+1))
			}

			alt := bsAlternative(text, html)

			var attrs []imap.BodyStructure
			for _, attachment := range traEmail.Attachments {
				attrs = append(attrs, bsAttachment(attachment.Filename, attachment.ContentType, uint32(len(attachment.Content)), "base64"))
			}
			bs := bsMixedWithAttachments(alt, options.BodyStructure.Extended, attrs...) // final BodyStructure (interface value)

			writer.WriteBodyStructure(bs)
		}
		if options.RFC822Size {
			emailContent := traEmail.BuildBytes(ctx, false)
			writer.WriteRFC822Size(cast.ToInt64(len(emailContent)))
		}
		if options.Flags {
			if email.IsRead == 1 {
				writer.WriteFlags([]imap.Flag{imap.FlagSeen})
			} else {
				writer.WriteFlags([]imap.Flag{})
			}
		}
		if options.InternalDate {
			writer.WriteInternalDate(email.CreateTime)
		}
		for _, section := range options.BodySection {
			if !section.Peek {
				detail.MakeRead(ctx, email.UeId, true)
			}
			emailContent := traEmail.BuildBytes(ctx, false)

			// Check first whether HEADER.FIELDS were requested
			if len(section.HeaderFields) > 0 || section.Specifier == imap.PartSpecifierHeader {
				var b bytes.Buffer
				fields := section.HeaderFields

				if fields == nil || len(fields) == 0 {
					// No fields specified: return all common headers
					fields = []string{
						"date", "subject", "from", "to", "cc", "message-id", "content-type",
					}
				}

				for _, field := range fields {
					fieldLower := strings.ToLower(field)
					switch fieldLower {
					case "date":
						fmt.Fprintf(&b, "Date: %s\r\n", email.CreateTime.Format(time.RFC1123Z))
					case "subject":
						fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", email.Subject))
					case "from":
						if email.FromName != "" {
							fmt.Fprintf(&b, "From: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", email.FromName), email.FromAddress)
						} else {
							fmt.Fprintf(&b, "From: %s\r\n", email.FromAddress)
						}
					case "sender":
						if email.FromName != "" {
							fmt.Fprintf(&b, "Sender: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", email.FromName), email.FromAddress)
						} else {
							fmt.Fprintf(&b, "Sender: %s\r\n", email.FromAddress)
						}
					case "reply-to":
						if len(traEmail.ReplyTo) > 0 && traEmail.ReplyTo[0].EmailAddress != "" {
							if traEmail.ReplyTo[0].Name != "" {
								fmt.Fprintf(&b, "Reply-To: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", traEmail.ReplyTo[0].Name), traEmail.ReplyTo[0].EmailAddress)
							} else {
								fmt.Fprintf(&b, "Reply-To: %s\r\n", traEmail.ReplyTo[0].EmailAddress)
							}
						}
					case "to":
						toStr := traEmail.BuildTo2String()
						if toStr != "" {
							fmt.Fprintf(&b, "To: %s\r\n", toStr)
						}
					case "cc":
						if len(traEmail.Cc) > 0 {
							fmt.Fprintf(&b, "Cc: %s\r\n", traEmail.BuildCc2String())
						}
					case "bcc":
						if len(traEmail.Bcc) > 0 {
							fmt.Fprintf(&b, "Bcc: %s\r\n", traEmail.BuildBcc2String())
						}
					case "message-id":
						if email.MsgID != "" {
							fmt.Fprintf(&b, "Message-ID: <%s>\r\n", email.MsgID)
						} else {
							fmt.Fprintf(&b, "Message-ID: <%d@%s>\r\n", email.Id, config.Instance.Domain)
						}
					case "content-type":
						args := strings.SplitN(string(emailContent), "\r\n", 3)
						if len(args) >= 2 {
							fmt.Fprintf(&b, "%s%s\r\n", args[0], args[1])
						}
					case "references", "in-reply-to", "thread-topic", "thread-index", "x-priority", "x-mailer", "x-android-message-id":
						// These headers are not stored yet, skip them
					default:
						// Ignore any other unknown header
					}
				}

				// Append the terminating empty line
				b.WriteString("\r\n")

				bodyWriter := writer.WriteBodySection(section, cast.ToInt64(b.Len()))
				bodyWriter.Write(b.Bytes())
				bodyWriter.Close()
			} else if section.Specifier == imap.PartSpecifierNone || section.Specifier == imap.PartSpecifierText {
				if len(section.Part) >= 1 {
					// Fetch the content of the requested part
					partContent := traEmail.BuildPart(ctx, section.Part)
					if partContent != nil {
						bodyWriter := writer.WriteBodySection(section, cast.ToInt64(len(partContent)))
						bodyWriter.Write(partContent)
						bodyWriter.Close()
					} else {
						// Part does not exist, return empty
						bodyWriter := writer.WriteBodySection(section, 0)
						bodyWriter.Close()
					}
				} else {
					bodyWriter := writer.WriteBodySection(section, cast.ToInt64(len(emailContent)))
					bodyWriter.Write(emailContent)
					bodyWriter.Close()
				}
			}
		}
		writer.Close()
	}
}
