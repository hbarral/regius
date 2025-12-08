package mailer

import (
	"errors"
	"testing"
)

func TestMail_SendSMTPMessage(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	err := mailer.SendSMTPMessage(msg)
	if err != nil {
		t.Error(err)
	}
}

func TestMail_SendUsingChan(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	mailer.Jobs <- msg
	result := <-mailer.Results
	if result.Error != nil {
		t.Error(errors.New("failed to send over channel"))
	}

	msg.To = "not_an_email_address"
	mailer.Jobs <- msg
	result = <-mailer.Results
	if result.Error == nil {
		t.Error(errors.New("no error received with invalid address"))
	}
}

func TestMail_SendUsingAPI(t *testing.T) {
	msg := Message{
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	mailer.API = "unknown"
	mailer.APIKey = "abc123"
	mailer.APIUrl = "https://www.fake.com"

	err := mailer.SendUsingAPI(msg, "unknown")
	if err == nil {
		t.Error(err)
	}

	mailer.API = ""
	mailer.APIKey = ""
	mailer.APIUrl = ""
}

func TestMail_buildHTMLMessage(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	_, err := mailer.buildHTMLMessage(msg)
	if err != nil {
		t.Error(err)
	}
}

func TestMail_buildPlainMessage(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	_, err := mailer.buildPlainTextMessage(msg)
	if err != nil {
		t.Error(err)
	}
}

func TestMail_send(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	err := mailer.Send(msg)
	if err != nil {
		t.Error(err)
	}

	mailer.API = "unknown"
	mailer.APIKey = "abc123"
	mailer.APIUrl = "https://fake.com"

	err = mailer.Send(msg)
	if err == nil {
		t.Error("did not get an error when we should have")
	}

	mailer.API = ""
	mailer.APIKey = ""
	mailer.APIUrl = ""
}

func TestMail_ChooseAPI(t *testing.T) {
	msg := Message{
		From:        "a@a.com",
		FromName:    "Joe",
		To:          "b@b.com",
		Subject:     "test",
		Template:    "test",
		Attachments: []string{"./testdata/mail/test.html.template"},
	}

	mailer.API = "unknown"
	err := mailer.ChooseAPI(msg)
	if err == nil {
		t.Error(err)
	}
}
