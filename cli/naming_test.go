package cli

import "testing"

func TestPascalIdent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hyphenated", "send-welcome-email", "SendWelcomeEmail"},
		{"underscored", "send_welcome_email", "SendWelcomeEmail"},
		{"camel", "sendWelcomeEmail", "Sendwelcomeemail"},
		{"single", "post", "Post"},
		{"mixed_case_single", "Post", "Post"},
		{"upper_snake", "STRIPE_PAYMENT", "StripePayment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pascalIdent(c.in); got != c.want {
				t.Fatalf("pascalIdent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSnakeIdent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hyphenated", "send-welcome-email", "send_welcome_email"},
		{"underscored", "send_welcome_email", "send_welcome_email"},
		{"single", "post", "post"},
		{"mixed_case", "MyPost", "mypost"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := snakeIdent(c.in); got != c.want {
				t.Fatalf("snakeIdent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestEnvIdent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hyphenated", "stripe-payment", "STRIPE_PAYMENT"},
		{"underscored", "stripe_payment", "STRIPE_PAYMENT"},
		{"single", "hook", "HOOK"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := envIdent(c.in); got != c.want {
				t.Fatalf("envIdent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
