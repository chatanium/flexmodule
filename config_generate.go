package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"time"

	"github.com/thirdscam/chatanium-flexmodule/internal/host"
	wire "github.com/thirdscam/chatanium-flexmodule/proto/core-v2"
	runtime "github.com/thirdscam/chatanium-flexmodule/runtime"
	"google.golang.org/protobuf/encoding/protojson"
)

// generateConfig fills only an explicitly approved installation. Host settings,
// finite limits and approvals are operator inputs, never inferred from an image
// label or widened to make a module start. The resulting document is re-decoded
// through the production validator before any bytes are emitted.
func generateConfig(args []string) error {
	flags := flag.NewFlagSet("generate-config", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("base", "", "absolute host configuration with modules:[]")
	moduleConfig := flags.String("module-config", "", "absolute module-scoped JSON file")
	approval := flags.String("approval", "", "absolute protobuf-JSON Grants file")
	image := flags.String("image", "", "real immutable registry/repository@sha256 digest")
	id := flags.String("module-id", "", "stable operator-assigned module ID")
	entrypoint := flags.String("entrypoint", "", "approved image executable absolute path")
	uid := flags.Uint("uid", 0, "approved nonzero module UID")
	gid := flags.Uint("gid", 0, "approved nonzero module GID")
	configRevision := flags.String("config-revision", "", "operator configuration revision")
	approvalRevision := flags.String("approval-revision", "", "operator approval revision")
	socket := flags.String("supervisor-socket", "", "absolute real supervisor socket for image verification")
	supervisorUID := flags.Int64("supervisor-uid", -1, "trusted supervisor UID")
	maxBytes := flags.Int("supervisor-max-bytes", 0, "supervisor packet bound")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *uid == 0 || *gid == 0 || uint64(*uid) > 1<<32-1 || uint64(*gid) > 1<<32-1 || !canonicalPath(*socket) || *supervisorUID < 0 || *supervisorUID > 1<<32-1 || *maxBytes <= 0 {
		return errors.New("invalid config generation arguments")
	}
	raw, err := readOperatorFile(*base)
	if err != nil {
		return err
	}
	decoded, err := runtime.DecodeConfig(raw)
	if err != nil || len(decoded.Modules) != 0 {
		return errors.New("base must be a valid host configuration with no installed modules")
	}
	configRaw, err := readOperatorFile(*moduleConfig)
	if err != nil {
		return err
	}
	approvalRaw, err := readOperatorFile(*approval)
	if err != nil {
		return err
	}
	if rejectDuplicateJSON(configRaw) != nil || rejectDuplicateJSON(approvalRaw) != nil {
		return errors.New("invalid module config or approval JSON")
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(raw, &document) != nil {
		return errors.New("invalid base document")
	}
	module := struct {
		ModuleID           string                                 `json:"module_id"`
		Image              string                                 `json:"image"`
		Entrypoint         []string                               `json:"entrypoint"`
		Args               []string                               `json:"args"`
		UID                uint                                   `json:"uid"`
		GID                uint                                   `json:"gid"`
		Config             json.RawMessage                        `json:"config"`
		Approval           json.RawMessage                        `json:"approval"`
		CredentialBindings []runtime.ProvisionedCredentialBinding `json:"credential_bindings"`
		ConfigRevision     string                                 `json:"config_revision"`
		ApprovalRevision   string                                 `json:"approval_revision"`
	}{*id, *image, []string{*entrypoint}, []string{}, *uid, *gid, configRaw, approvalRaw, []runtime.ProvisionedCredentialBinding{}, *configRevision, *approvalRevision}
	document["modules"], err = json.Marshal([]any{module})
	if err != nil {
		return errors.New("config generation failed")
	}
	output, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return errors.New("config generation failed")
	}
	if _, err = runtime.DecodeConfig(output); err != nil {
		return errors.New("generated installation rejected: real digest, nonzero IDs, exact approval profile and finite host limits required")
	}
	// Verify the exact image really exists in the supervisor's local image store.
	// Requirements remain untrusted: the separately supplied approval is unchanged.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(decoded.Limits.DockerAPIDeadline))
	defer cancel()
	client, err := host.Dial(ctx, host.ClientConfig{SocketPath: *socket, SupervisorUID: uint32(*supervisorUID), MaxMessageBytes: *maxBytes, OperationTimeout: time.Duration(decoded.Limits.DockerAPIDeadline)})
	if err != nil {
		return errors.New("image verification supervisor unavailable")
	}
	defer client.Close()
	declaration, err := client.ImageManifest(ctx, *image)
	if err != nil {
		return errors.New("exact immutable image declaration unavailable")
	}
	manifest := new(wire.ModuleManifest)
	if protojson.Unmarshal(declaration, manifest) != nil || manifest.ProtocolMajor != 2 {
		return errors.New("image has no strict canonical v2 manifest")
	}
	if _, err = os.Stdout.Write(append(output, '\n')); err != nil {
		return errors.New("config output failed")
	}
	return nil
}
