package main

import (
    "net/http"
    "testing"
    "time"
)

// A verified artifact may occupy the process-wide memory reservation until
// the peer receives (or abandons) it. Both bound management listeners must
// enforce the same finite write deadline, independent of idle/read timeouts.
func TestControlServerTimeoutsBoundVerifiedDownloadLifetime(t *testing.T) {
    handler:=http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){w.WriteHeader(http.StatusNoContent)})
    for _,addr:=range []string{"127.0.0.1:18080","127.0.0.1:18443"} {
        srv:=newControlHTTPServer(addr,handler)
        if srv.Addr!=addr || srv.Handler==nil {
            t.Fatalf("%s: control listener misconfigured",addr)
        }
        if srv.WriteTimeout!=5*time.Minute || srv.WriteTimeout<=srv.IdleTimeout {
            t.Fatalf("%s: missing or mismatched finite response deadline: %v",addr,srv.WriteTimeout)
        }
        if srv.ReadHeaderTimeout!=5*time.Second || srv.IdleTimeout!=60*time.Second {
            t.Fatalf("%s: previous read/idle guards changed",addr)
        }
    }
}
