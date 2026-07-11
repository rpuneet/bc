# Manage Secrets

Secrets are credentials — API keys, tokens, cloud profiles — kept in an encrypted vault and referenced from agent environment variables so they never live in plain text.

## Overview

The vault is an encrypted store. Values are sealed with AES-256-GCM under a key derived by PBKDF2-SHA256, and the ciphertext lives in `~/.mycel/secrets.vault`. A repo can add its own overrides in a scoped store; when the same name exists in both, the repo-scoped value wins. Agents never read raw secrets from configuration — they reference vault entries by name and mycel resolves them at spawn.

## Store a Secret

Use the `mycel secret` commands. A value can come from a flag, an environment variable, a file, or standard input:

```bash
# From stdin (keeps the value out of shell history)
echo "sk-..." | mycel secret set ANTHROPIC_API_KEY

# From an existing environment variable
mycel secret set GH_TOKEN --from-env GH_TOKEN

# From a file, with a description
mycel secret set DEPLOY_KEY --from-file ./deploy.key --desc "CD signing key"
```

List and inspect what is stored — metadata only, never values, unless you explicitly reveal one:

```bash
mycel secret list                 # names + descriptions, no values
mycel secret show GH_TOKEN        # metadata (created, updated, description)
mycel secret show GH_TOKEN --reveal
mycel secret delete GH_TOKEN
```

Add `--workspace` to `set` or `delete` to store or remove a repo-scoped override instead of the global entry.

## Reference a Secret from an Agent

Each agent carries an `env` map that mycel injects into its session. Set any value to `${secret:NAME}` to pull it from the vault at spawn:

```
AWS_PROFILE = ${secret:AWS_PROFILE}
AWS_REGION  = ${secret:AWS_REGION}
```

The `${secret:NAME}` placeholder is resolved only when the agent starts — the reference is what's stored on the agent, and the decrypted value never touches configuration or disk. Names reserved to the runtime (the `MYCEL_` prefix) are protected and skipped.

### From the web UI

The **Create Agent** modal and the **Agent Detail** settings panel both include an environment-variable editor with secret autocomplete. Type `${` in a value field (or use the key button) and a dropdown of vault secret names appears; pick one and it inserts the full `${secret:NAME}` reference. Changes to an existing agent's env take effect on its next restart.

### From the API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/agents/{name}/env` | List an agent's env vars (references shown verbatim) |
| PUT | `/api/agents/{name}/env` | Replace an agent's env vars; applies on next restart |

## Worked Example: AWS Bedrock

Reaching a Bedrock-backed model needs AWS credentials in the agent's environment without hardcoding them. Store the profile and region as secrets, then reference them from the agent that talks to Bedrock:

```bash
mycel secret set AWS_PROFILE --value "default"
mycel secret set AWS_REGION  --value "us-west-2"
```

```
AWS_PROFILE = ${secret:AWS_PROFILE}
AWS_REGION  = ${secret:AWS_REGION}
```

At spawn, mycel resolves both references and injects them into that agent's session, so the model provider picks up the credentials while the values stay sealed in the vault. The same pattern works for any provider that reads its configuration from environment variables.

> Tip: keep provider keys in the global vault and only use `--workspace` overrides when a specific repo needs a different value.
