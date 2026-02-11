# Tabula Spec Index (Code-First)

This MVP uses code + tests as canonical spec.

## Mode contract

- Modes: `prompt`, `discussion`
- Activation: any valid `text_artifact`, `image_artifact`, or `pdf_artifact`
- Deactivation: `clear_canvas`

## Event kinds

- `text_artifact`
- `image_artifact`
- `pdf_artifact`
- `clear_canvas`

## Scenario mapping

1. CLI usage modes
- `tests/bdd/test_cli_usage_modes.py::test_given_schema_mode_when_invoked_then_prints_contract`
- `tests/bdd/test_cli_usage_modes.py::test_given_canvas_mode_when_invoked_then_calls_ui_runner`
- `tests/bdd/test_cli_usage_modes.py::test_given_canvas_mode_without_window_dependency_then_shows_install_hint`
- `tests/bdd/test_cli_usage_modes.py::test_given_missing_event_file_when_checking_then_nonzero_exit`
- `tests/bdd/test_cli_usage_modes.py::test_given_valid_event_file_when_checking_then_passes`
- `tests/bdd/test_cli_usage_modes.py::test_given_invalid_event_lines_when_checking_then_reports_all_errors`
- `tests/bdd/test_cli_usage_modes.py::test_given_bootstrap_mode_when_invoked_then_project_is_prepared`
- `tests/bdd/test_cli_usage_modes.py::test_given_markdown_mvp_mode_when_invoked_then_cli_returns_workflow_status`

2. Prompt/discussion mode transitions
- `tests/bdd/test_mode_and_event_scenarios.py::test_given_prompt_mode_when_artifact_event_arrives_then_mode_switches_to_discussion`
- `tests/bdd/test_mode_and_event_scenarios.py::test_given_discussion_mode_when_clear_canvas_arrives_then_mode_switches_back_to_prompt`
- `tests/gui/test_window_mode_switch.py::test_window_mode_switches_prompt_discussion_prompt`

3. Strict event validation (all event kinds)
- `tests/unit/test_events.py`
- `tests/bdd/test_mode_and_event_scenarios.py::test_given_invalid_event_payload_when_parsed_then_strict_validation_rejects`

4. Watcher + stream behavior (including malformed lines)
- `tests/integration/test_watcher.py::test_watcher_reads_appended_events_and_skips_invalid`
- `tests/bdd/test_watcher_and_codex_mock.py::test_given_codex_emits_events_when_canvas_polls_then_only_new_lines_are_processed`
- `tests/bdd/test_watcher_and_codex_mock.py::test_given_malformed_stream_when_canvas_polls_then_parser_errors_are_kept_and_stream_continues`
- `tests/bdd/test_watcher_and_codex_mock.py::test_given_full_mode_cycle_when_events_reduce_state_then_prompt_discussion_prompt`

5. UI poll loop behavior with mocked watcher
- `tests/gui/test_window_mode_switch.py::test_window_poll_once_uses_watcher_results`

6. Project bootstrap protocol (`AGENTS.md`, git init, binary ignore)
- `tests/bdd/test_protocol_bootstrap_and_commit.py::test_given_new_project_when_bootstrapped_then_git_agents_and_binary_ignores_are_created`
- `tests/bdd/test_protocol_bootstrap_and_commit.py::test_given_existing_agents_when_bootstrapped_then_protocol_block_is_upserted_without_losing_custom_text`

7. Markdown-only commit policy
- `tests/bdd/test_protocol_bootstrap_and_commit.py::test_given_markdown_changes_when_committing_then_only_markdown_path_is_staged_and_committed`
- `tests/bdd/test_protocol_bootstrap_and_commit.py::test_given_no_markdown_changes_when_committing_then_no_commit_is_created`

8. Codex `project/global` markdown MVP flow
- `tests/bdd/test_markdown_mvp_workflow.py::test_given_project_mode_when_running_markdown_mvp_then_two_codex_rounds_render_pdf_and_markdown_commit`
- `tests/bdd/test_markdown_mvp_workflow.py::test_given_global_mode_when_running_markdown_mvp_then_codex_is_invoked_with_skip_repo_check`
- `tests/bdd/test_markdown_mvp_workflow.py::test_given_missing_pandoc_when_running_markdown_mvp_then_workflow_fails_before_commit`
- `tests/bdd/test_markdown_mvp_workflow.py::test_build_codex_command_supports_project_and_global_modes`

9. Optional real-tool integration
- `tests/integration/test_real_optional_tools.py::test_real_pandoc_render_markdown_to_pdf`
- `tests/integration/test_real_optional_tools.py::test_real_codex_exec_writes_output_file`
