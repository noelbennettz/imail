package consts

const (
	// EmailTypeSend outgoing mail
	EmailTypeSend int8 = 1
	// EmailTypeReceive incoming mail
	EmailTypeReceive int8 = 0

	// EmailStatusWait 0 = not sent
	EmailStatusWait int8 = 0

	// EmailStatusSent 1 = sent
	EmailStatusSent int8 = 1

	// EmailStatusFail 2 = send failed
	EmailStatusFail int8 = 2

	// EmailStatusDel 3 = deleted
	EmailStatusDel int8 = 3

	// EmailStatusDrafts draft
	EmailStatusDrafts int8 = 4

	// EmailStatusJunk junk mail
	EmailStatusJunk int8 = 5

	// EmailStatusDeliveryPending marks an outbound audit whose delivery result is not final.
	EmailStatusDeliveryPending int8 = 6
)
