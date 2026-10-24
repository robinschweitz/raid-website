package v1

import "testing"

func TestPermittedPushEndpoint(t *testing.T) {
    cases := []struct{ endpoint string; allowed bool }{
        {"https://fcm.googleapis.com/fcm/send/abc", true},
        {"https://push.services.mozilla.com/wpush/v2/abc", true},
        {"https://web.push.apple.com/Qabc", true},
        {"http://push.services.mozilla.com/wpush/v2/a", false},
        {"https://127.0.0.1/private", false},
        {"https://fcm.googleapis.com.attacker.example/x", false},
        {"https://user@fcm.googleapis.com/x", false},
        {"https://fcm.googleapis.com:8443/x", false},
    }
    for _, tt := range cases {
        if got := permittedPushEndpoint(tt.endpoint); got != tt.allowed {
            t.Errorf("endpoint %q: got %v, want %v", tt.endpoint, got, tt.allowed)
        }
    }
}
