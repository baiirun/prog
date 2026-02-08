# Agent Orchestrator (Stream of Consciousness)

Goal
- Lightweight orchestrator for 5-10 agents.
- Agents are black boxes (Codex/Claude Code manage tools/resources).
- Orchestrator focuses on scheduling, orchestration, and review only.
- Built for real production work, not vibe coding: quality, craft, reliability are axioms.

High-level parallels
- Agent pools ~ thread pools.
- Work items ~ async tasks/futures.
- Work stealing for throughput and uptime.
- Joining/merging related work.

Core principles
- Avoid runtime locking/coordination. Prevent conflicts at planning time.
- Serialize tasks likely to collide; parallelize tasks that are independent.
- There is an optimal parallelism level; tune to maximize throughput while minimizing conflicts.

Planning
- Planners/leads decompose work into tasks, define dependencies, and identify likely collisions.
- Design and review are bottlenecks; planners and reviewers likely need to scale with implementers.
- Colliding work is ordered in the plan rather than coordinated at runtime.

DAG in prog
- prog is the source of truth for tasks.
- Store dependency DAG and conflict/ordering DAG (or conflict groups) as task metadata.

Scheduling
- Lock-free runtime.
- Scheduler assigns ready, non-conflicting tasks from the DAG order.
- Work stealing is from other agents' inboxes (tokio-style), not from a global unassigned pool.

Daemon + inboxes
- Orchestrator runs as a daemon, project-independent like prog.
- It can coordinate multiple folders/projects at once; tasks carry project/workspace references.
- Each agent has a daemon-resident inbox deque.
- Daemon pushes work into agent inboxes; agents poll their inbox for assignments.
- Work stealing is coordinated by the daemon when it rebalances inboxes.
- Inboxes live in the daemon to allow atomic reassignment without file contention.

Agent pool (volunteer model)
- Agents are started externally and register with the daemon.
- The daemon schedules only across the currently registered pool.
- Agents can deregister explicitly, or implicitly via missed heartbeats.
- Heartbeat: agents send a heartbeat every N seconds; daemon marks stale if no heartbeat within TTL (e.g., N=30s, TTL=90s).
- DEREGISTER: agent sends a final message; daemon marks it offline and releases its inbox.
- Reassignment policy on agent loss:
  - If task had no reported progress: requeue to todo.
  - If task had progress: mark blocked + require human confirmation to reassign, unless policy allows auto-requeue.

Messaging protocol (minimal, daemon <-> agent)
- Goal: push as little over the wire as possible; agents read task context from prog using task_id.
- REGISTER {agent_id}: agent joins the pool.
- HEARTBEAT {agent_id, status, task_id?}: liveness + current state.
- ASSIGN {agent_id, task_id, role}: daemon pushes a task into the agent inbox.
- STATUS {agent_id, status, task_id, note?}: progress or blocked state.
- RESULT {agent_id, task_id, summary_ref?}: completion signal (details stored in prog).
- DEREGISTER {agent_id, reason}: agent leaves the pool.

Agent ID mechanics
- Agents generate their own IDs as short 6-char base36 strings.
- If collision, daemon responds with ID_IN_USE and agent retries.
- IDs are used for inbox routing, heartbeats, and dashboard visibility.

Agent bootstrap prompt (mirrors prog prime)
# Agent Orchestrator Context
This system uses the daemon + prog for cross-session orchestration.
Agents are volunteers: register with the daemon and wait for assignments.

## Starting Work
When you receive an assignment:
1. ASSIGN gives you task_id + role
2. Read task details directly from prog:
   - prog show <task_id>
   - prog context -c X --summary (scan first)
3. Load project policy (AGENTS.md, CONTRIBUTING.md, etc.)
4. Follow role instructions (implementer/reviewer/planner)

Load relevant context only. Do not load everything.

## SESSION CLOSE PROTOCOL
Before ending ANY agent session, you MUST:
1. Emit progress status:
   - STATUS {working|verifying|blocked|review_ready}
2. Log progress in prog:
   - prog log <id> "What you accomplished"
3. Reopen tasks if needed:
   - prog open <id>
4. Hand off next steps:
   - prog append <id> "Next steps: ..."

Never end a session without updating state and logging.

## Core Rules (Daemon + prog)
- Only the daemon changes task state in prog.
- Agents only log progress/blockers/results.
- Always run required checks before reporting review_ready.
- If blocked or uncertain, emit STATUS blocked and wait.

## Agent Loop
- Register with daemon -> poll inbox -> ASSIGN -> execute -> verify -> report -> repeat
- If blocked: emit blocked status and pause until human input.

## Verification
- Required checks come from project policy + task brief.
- Run checks before reporting review_ready.
- If checks fail, fix and re-run.

## Essential Commands
# Work intake
prog show <id>           # task details + suggested concepts
prog context -c X        # load learnings
prog context -c X --summary

# Progress logging
prog log <id> "message"
prog open <id>             # reopen if needed
prog append <id> "Next steps..."

# Status (daemon)
STATUS blocked | working | verifying | review_ready

## Current State
Use prog ready to see available work.
Use daemon dashboard to see pool + assignments.

prog integration
- Daemon reads/updates prog directly (no cache required initially).
- Review gating in prog: todo -> in_progress -> review -> done (with failed/blocked as needed).
- Agents never mark done; only daemon does after review passes.

Human in the loop
- Agents run in terminal windows; human checks periodically.
- Agents emit a simple status signal (blocked) when stuck; human intervenes directly.
- Once unblocked, agent resumes the loop.

Dashboard
- Minimal status view for humans: agent status, tasks in queue, review backlog, blocked tasks.

Stacked diffs via commit stacks
- One branch per chain; each task = one commit.
- Review commit-by-commit; merge after stack approved (or per-prefix).
- History rewriting is encouraged to keep task commits clean and ordered.
- Once a task enters review, either lock earlier commits or invalidate review on rewrite.

Worktree/branch mapping
- Daemon tracks agent -> worktree/branch -> current task for isolation and correct routing.

Open questions
- How to encode conflict/ordering metadata in prog tasks (explicit edges vs groups).
- Review policy detail (1 reviewer vs 2 vs human-required by risk).
- How to tune optimal parallelism dynamically.

Lifecycle (formal)
- System phases: intake -> plan -> schedule -> execute -> verify -> review -> done.
- Task states (prog): todo -> in_progress -> review -> done (plus blocked/failed).

State machine (system lifecycle)
```
          +------------------+
          |  idea / intake   |
          +---------+--------+
                    |
                    v
          +------------------+
          |  design / plan   |
          | (human or agent) |
          +---------+--------+
                    |
                    v
          +------------------+
          |  prog: todo      |
          +---------+--------+
                    |
                    v
          +------------------+
          |  in_progress     |
          +----+--------+----+
               |        |
               |        +-----------------------------------------------+
               v                                                        |
     +-------------------- inner agent loop --------------------+       |
     |  +-------------+   +-------------+   +-----------+        |       |
     |  |  executing  |<->|  verifying  |<->|  blocked  |        |       |
     |  +------+------+   +------+------+   +-----+-----+        |       |
     |         |                |                |              |       |
     |         +----------------+----------------+              |       |
     |                          |                               |       |
     |                          v                               |       |
     |                    +-----------+                         |       |
     |                    | report    |-------------------------+       |
     |                    |  ready    |                                 |
     |                    +-----+-----+                                 |
     +---------------------------|--------------------------------------+
                                 v
                          +-------------+
                          | prog:review |
                          +------+------+
                                 | 
                                 +--------------------+
                                 |                    |
                                 v                    v
                             +-------+        +------------------+
                             | done  |        | changes requested|
                             +-------+        +--------+---------+
                                                      |
                                                      v
                                                (back into inner loop)
```

Agent state definitions
- idle: no task assigned; agent is available.
- ready: agent requests work from daemon.
- assigned: daemon assigns a task; agent acknowledges.
- context_loaded: agent reads task brief, project rules, and required checks.
- working: agent edits/implements changes.
- verifying: agent runs required checks; loops until pass or blocked.
- blocked: agent cannot proceed; emits blocked status and awaits human input.
- reporting: agent submits results, summary, and verification outcomes.
- revising: agent applies changes requested by review.

Notes on inner loop
- Working, verifying, and blocked are a tight loop; an agent can move between any of them repeatedly.
- After verification passes, the agent can move to reporting and mark review-ready.
- If a reviewer requests changes, the agent enters revising and returns to verifying.
