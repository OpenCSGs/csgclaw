# CSGClaw OpenAPI

[control-plane.json](control-plane.json) is the OpenAPI 3.1.1 definition for Agent, Runtime, model-provider, Skill, MCP, knowledge-base and template operations. It includes protocol proxy media types and existing credential requirements.

See [the control-plane API guide](../../docs/tech/control-plane-api.md) for request examples and update semantics.

After changing the definition, run `pnpm --dir web/app generate:api`. Keep the generated TypeScript, HTTP implementation and route coverage test consistent. The frontend continues using its existing transport for authentication, instance prefixes, cancellation and streaming.
