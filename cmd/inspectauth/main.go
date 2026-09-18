package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func main() {
	ctx := context.Background()
	record, err := praxiscrypto.LoadBootstrapRecord(os.Getenv("PRAXIS_BOOTSTRAP_RECORD"))
	if err != nil { panic(err) }
	reg, err := praxiscrypto.NewFirstPartyBootstrapRegistry(); if err != nil { panic(err) }
	wrapper, err := reg.Open(ctx, record); if err != nil { panic(err) }
	keys := praxiscrypto.NewProviderRegistry(); if err := keys.Register(record.ProviderID, wrapper); err != nil { panic(err) }
	service, err := keys.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{}); if err != nil { panic(err) }
	db, err := state.OpenSQLite(ctx, os.Getenv("PRAXIS_DB")); if err != nil { panic(err) }; defer db.Close()
	store := state.New(db)
	for _, ns := range []string{"authority_generation", "authority_decision", "authority_request", "work_plan_acceptance", "goal_baseline"} {
		recs, err := store.ListSecureBlobs(ctx, ns, time.Now().UTC()); if err != nil { panic(err) }
		fmt.Printf("NAMESPACE %s COUNT %d\n", ns, len(recs))
		for _, rec := range recs {
			payload, err := service.Open(ctx, rec.Envelope, state.SecureBlobAAD(rec.Namespace, rec.ObjectID, rec.ObjectVersion, rec.ObjectDigest)); if err != nil { panic(err) }
			fmt.Printf("RECORD %s/%s %s\n", rec.ObjectID, rec.ObjectVersion, rec.ObjectDigest)
			if ns == "authority_generation" {
				var g contracts.AuthorityGeneration; if err := json.Unmarshal(payload, &g); err != nil { panic(err) }
				fmt.Printf("GENERATION ref=%s version=%s digest=%s principal=%s kind=%s scope=%s state=%s\n", g.Ref, g.Version, g.Digest, g.Principal.ID, g.Principal.Kind, g.Scope, g.State)
			}
		}
	}
}
