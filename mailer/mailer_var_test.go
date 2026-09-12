package mailer

var mailer = Mail{
	Domain:      "localhost",
	Templates:   "./testdata/mail",
	Host:        "localhost",
	Port:        1026,
	Encryption:  "none",
	FromAddress: "a@a.com",
	FromName:    "John",
	Jobs:        make(chan Message, 1),
	Results:     make(chan Result, 1),
}
