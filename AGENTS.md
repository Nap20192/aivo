# AGENTS.md

## Project

**AIVO RMS** is an open-source, AI-powered restaurant management system SaaS.

Goal: help restaurants manage operations with simple, reliable software and useful AI automation.

## Product scope

Core modules:

- Restaurant onboarding and multi-tenant SaaS accounts
- Menu, modifiers, allergens, and pricing
- Tables, reservations, orders, and kitchen display workflows
- Inventory, purchasing, waste tracking, and stock alerts
- Staff roles, shifts, payroll exports, and permissions
- Customer profiles, loyalty, feedback, and marketing
- Analytics, forecasts, and AI recommendations
- Integrations for POS, payments, delivery, accounting, and messaging

## Working rules for agents

- Keep the codebase boring and maintainable.
- Prefer existing code, standard libraries, and framework defaults before adding dependencies.
- Do not add speculative abstractions or placeholder systems.
- Treat tenant isolation, permissions, payments, and customer data as security-critical.
- Validate all external input at boundaries.
- Never commit secrets, API keys, credentials, or real customer data.
- Add the smallest useful test for non-trivial behavior.
- Update docs when behavior, commands, setup, or configuration changes.

## AI feature rules

AI must assist, not silently control business-critical actions.

- Show confidence, reasoning summary, or source data where useful.
- Require confirmation for destructive or financial actions.
- Log AI-generated operational recommendations.
- Keep prompts and model calls tenant-safe.
- Do not train or leak data across tenants.

## Commands

This repository is currently being initialized. When implementation exists, keep this section updated with the real commands:

```bash
# install
# test
# lint
# run
```
