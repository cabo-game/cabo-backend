# Cabo Backend — Claude Code Instructions

## Project

This repository contains the backend service of the Cabo system.

Cabo is a multi-service project. The backend is developed independently
from the other services and has its own Git repository.

## Shared BMAD Context

The shared BMAD project repository is located at:

../cabo-bmad

The shared BMAD installation, agents, skills, workflows, and project-level
artifacts are maintained there.

When working on this service, use the BMAD methodology and consult the
shared BMAD artifacts when relevant.

Important shared locations:

- `../cabo-bmad/_bmad/` — BMAD framework, modules, workflows, and configuration
- `../cabo-bmad/_bmad-output/` — BMAD-generated project artifacts and shared
  project knowledge

Do not copy the BMAD installation into this repository.
Do not modify files under `../cabo-bmad/_bmad/` from this repository.

## Shared Project Context

Before making changes that affect system-level behavior, consult the
relevant artifacts in `../cabo-bmad/_bmad-output/`.

This includes changes involving:

- system architecture
- service boundaries
- game rules
- GameState
- API contracts
- WebSocket contracts
- authentication/authorization
- database ownership
- cross-service communication
- AI/backend interaction
- architectural decisions

Do not create duplicate system-level artifacts in this repository.

## BMAD Development

Follow the BMAD methodology for development work.

When a BMAD workflow or skill is appropriate for the task, use it rather
than inventing an alternative development process.

For significant changes:

1. Understand the relevant project context.
2. Identify affected requirements and architecture.
3. Check existing decisions and contracts.
4. Plan the implementation.
5. Implement the change.
6. Test the implementation.
7. Review the result against the requirements and architecture.

## Backend Responsibility

The backend is responsible for the authoritative game state and server-side
game logic.

Client-provided state must not be treated as authoritative.

All player actions must be validated by the backend before they can mutate
authoritative game state.

## Cross-Service Changes

If a backend change requires changes to another Cabo service or changes a
shared contract:

1. Identify the impact.
2. Consult the shared BMAD context.
3. Update the relevant shared artifact/contract through the appropriate
   BMAD workflow.
4. Implement the backend changes.
5. Clearly identify the changes required by other services.

## Scope

Keep backend-specific implementation details in this repository.

Keep system-wide requirements, architecture, decisions, and contracts in
the shared BMAD project repository.