package account

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "account" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "account")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, want := range []string{"register", "send-code", "verify"} {
		if !subs[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

func TestRegisterRequiredFlags(t *testing.T) {
	for _, flag := range []string{"phone", "email", "first-name", "last-name"} {
		f := registerCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
			continue
		}
		ann := registerCmd.Flags().Lookup(flag).Annotations
		if _, ok := ann["cobra_annotation_bash_completion_one_required_flag"]; !ok {
			t.Errorf("flag %q should be required", flag)
		}
	}
}

func TestRegisterSmsOptInFlagNotRequired(t *testing.T) {
	f := registerCmd.Flags().Lookup("sms-opt-in")
	if f == nil {
		t.Fatal("missing flag \"sms-opt-in\"")
	}
	if _, ok := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]; ok {
		t.Error("flag \"sms-opt-in\" should not be required")
	}
}

func TestSendCodeRequiredFlags(t *testing.T) {
	// delivery-channel is required, not defaulted: the choice of "sms" is
	// itself the customer's MFA-delivery consent, so it must be explicit
	// rather than silently assumed when the caller omits the flag.
	for _, flag := range []string{"phone", "email", "delivery-channel"} {
		f := sendCodeCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
			continue
		}
		if _, ok := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]; !ok {
			t.Errorf("flag %q should be required", flag)
		}
	}
}

func TestVerifyRequiredFlags(t *testing.T) {
	for _, flag := range []string{"phone", "email", "code"} {
		f := verifyCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
			continue
		}
		if _, ok := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]; !ok {
			t.Errorf("flag %q should be required", flag)
		}
	}
}
