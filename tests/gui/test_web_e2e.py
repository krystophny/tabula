from __future__ import annotations

import base64
import json
import urllib.request
from pathlib import Path

import pytest

pytest.importorskip("playwright")
from playwright.sync_api import Page, TimeoutError as PlaywrightTimeoutError, expect

TEST_PASSWORD = "e2e-test-pw-42"
TERMINAL_READY_TIMEOUT = 15_000
MARKER_TIMEOUT = 20_000

_TERM_BUFFER_JS = """() => {
    const el = document.querySelector('.terminal-text');
    return el ? el.textContent : '';
}"""

_GARBLED_PATTERN_JS = r"""(text) => {
    const esc = (text.match(/\x1b/g) || []).length;
    if (esc > 3) return 'raw ANSI escapes found (' + esc + ')';
    const ctrl = (text.match(/[\x00-\x08\x0e-\x1f]/g) || []).length;
    if (ctrl > 3) return 'control chars found (' + ctrl + ')';
    const bracket = (text.match(/\[[\d;]*[A-Za-z]/g) || []).length;
    if (bracket > 5) return 'CSI-like fragments found (' + bracket + ')';
    return '';
}"""


def _login(page: Page, base_url: str) -> None:
    page.goto(base_url)
    page.wait_for_selector("#view-auth", state="visible", timeout=5_000)
    page.fill("#auth-password", TEST_PASSWORD)
    page.click("#auth-submit")
    page.wait_for_selector("#view-main", state="visible", timeout=10_000)


def _wait_terminal_ready(page: Page) -> None:
    page.wait_for_function(
        """() => {
            const el = document.querySelector('.terminal-text');
            return el && el.textContent.trim().length > 0;
        }""",
        timeout=TERMINAL_READY_TIMEOUT,
    )


def _wait_for_marker(page: Page, marker: str, timeout: int = MARKER_TIMEOUT) -> None:
    try:
        page.wait_for_function(
            """(marker) => {
                const el = document.querySelector('.terminal-text');
                return el && el.textContent.includes(marker);
            }""",
            arg=marker,
            timeout=timeout,
        )
    except PlaywrightTimeoutError as exc:
        buf = page.evaluate(_TERM_BUFFER_JS)
        tail = "\n".join(buf.splitlines()[-30:])
        raise AssertionError(
            f"marker '{marker}' not found in terminal output tail:\n{tail}"
        ) from exc


def _assert_no_garbled_text(page: Page) -> None:
    buf = page.evaluate(_TERM_BUFFER_JS)
    garbled = page.evaluate(_GARBLED_PATTERN_JS, buf)
    if garbled:
        tail = "\n".join(buf.splitlines()[-30:])
        raise AssertionError(f"garbled terminal output detected: {garbled}\n{tail}")


def _type_in_terminal(page: Page, text: str) -> None:
    page.click("#terminal-container")
    page.keyboard.type(text, delay=30)


def _send_terminal_raw(page: Page, text: str) -> None:
    page.evaluate(
        """(text) => {
            const app = window._tabulaApp;
            if (!app || typeof app.getState !== 'function') {
                throw new Error('tabula app state not available');
            }
            const ws = app.getState().terminalWs;
            if (!ws || ws.readyState !== WebSocket.OPEN) {
                throw new Error('terminal websocket is not open');
            }
            ws.send(text);
        }""",
        text,
    )


def _send_terminal_resize(page: Page, *, cols: int, rows: int) -> None:
    page.evaluate(
        """(payload) => {
            const app = window._tabulaApp;
            if (!app || typeof app.getState !== 'function') {
                throw new Error('tabula app state not available');
            }
            const ws = app.getState().terminalWs;
            if (!ws || ws.readyState !== WebSocket.OPEN) {
                throw new Error('terminal websocket is not open');
            }
            ws.send(JSON.stringify({
                type: 'resize',
                cols: payload.cols,
                rows: payload.rows,
            }));
        }""",
        {"cols": cols, "rows": rows},
    )


def _terminal_scroll_metrics(page: Page) -> dict:
    return page.evaluate(
        """() => {
            const el = document.getElementById('terminal-container');
            if (!el) throw new Error('terminal container not found');
            const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
            return {
                scrollTop: el.scrollTop,
                scrollHeight: el.scrollHeight,
                clientHeight: el.clientHeight,
                distanceFromBottom,
            };
        }"""
    )


def _wait_terminal_near_bottom(page: Page, max_distance: int = 32) -> None:
    page.wait_for_function(
        """(maxDistance) => {
            const el = document.getElementById('terminal-container');
            if (!el) return false;
            const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
            return distance <= maxDistance;
        }""",
        arg=max_distance,
        timeout=10_000,
    )


def _ensure_terminal_overflow(page: Page, *, min_extra_px: int = 200) -> None:
    metrics = _terminal_scroll_metrics(page)
    if metrics["scrollHeight"] > metrics["clientHeight"] + min_extra_px:
        return
    marker = "PLAYWRIGHT_SCROLL_OVERFLOW_READY"
    _send_terminal_raw(
        page,
        "for i in $(seq 1 1200); do echo PLAYWRIGHT_SCROLL_FILL_$i; done; "
        f"echo {marker}\n",
    )
    _wait_for_marker(page, marker)
    metrics = _terminal_scroll_metrics(page)
    assert metrics["scrollHeight"] > metrics["clientHeight"] + min_extra_px


def _scroll_terminal_to_top(page: Page) -> None:
    page.evaluate(
        """() => {
            const el = document.getElementById('terminal-container');
            if (!el) throw new Error('terminal container not found');
            el.scrollTop = 0;
            el.dispatchEvent(new Event('scroll', { bubbles: true }));
        }"""
    )


def _scroll_terminal_to_bottom(page: Page) -> None:
    page.evaluate(
        """() => {
            const el = document.getElementById('terminal-container');
            if (!el) throw new Error('terminal container not found');
            el.scrollTop = el.scrollHeight;
            el.dispatchEvent(new Event('scroll', { bubbles: true }));
        }"""
    )


def _compose_in_terminal(page: Page, text: str) -> None:
    page.click("#terminal-container")
    page.evaluate(
        """(text) => {
            const ta = document.querySelector('.terminal-input-capture');
            if (!ta) throw new Error('terminal input capture not found');
            ta.focus();
            if (typeof CompositionEvent === 'function') {
                ta.dispatchEvent(new CompositionEvent('compositionstart', { data: '' }));
                ta.value = text;
                ta.dispatchEvent(new CompositionEvent('compositionupdate', { data: text }));
                ta.dispatchEvent(new InputEvent('input', {
                    data: text,
                    inputType: 'insertCompositionText',
                    bubbles: true,
                    composed: true,
                }));
                ta.dispatchEvent(new CompositionEvent('compositionend', { data: text }));
                ta.dispatchEvent(new InputEvent('input', {
                    data: text,
                    inputType: 'insertFromComposition',
                    bubbles: true,
                    composed: true,
                }));
                return;
            }
            ta.value = text;
            ta.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
        }""",
        text,
    )


def _screenshot(page: Page, name: str, screenshot_dir: Path) -> None:
    page.screenshot(path=str(screenshot_dir / name))


def _mcp_tool_call(daemon_url: str, msg_id: int, name: str, arguments: dict) -> None:
    body = json.dumps(
        {
            "jsonrpc": "2.0",
            "id": msg_id,
            "method": "tools/call",
            "params": {"name": name, "arguments": arguments},
        }
    ).encode()
    req = urllib.request.Request(
        daemon_url,
        method="POST",
        headers={"Content-Type": "application/json"},
        data=body,
    )
    urllib.request.urlopen(req, timeout=5)


def _write_test_png(path: Path) -> None:
    png_b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7+X1cAAAAASUVORK5CYII="
    path.write_bytes(base64.b64decode(png_b64))


def _write_test_pdf(path: Path) -> None:
    path.write_bytes(
        b"%PDF-1.4\n"
        b"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
        b"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
        b"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n"
        b"trailer\n<< /Root 1 0 R >>\n%%EOF\n"
    )


def test_setup_password_flow(page: Page, base_url: str, screenshot_dir: Path) -> None:
    page.goto(base_url)
    page.wait_for_selector("#view-auth", state="visible", timeout=5_000)

    expect(page.locator("#auth-subtitle")).to_contain_text("Set up")
    expect(page.locator("#auth-submit")).to_have_text("Set Password")
    _screenshot(page, "auth_setup.png", screenshot_dir)

    page.fill("#auth-password", TEST_PASSWORD)
    page.click("#auth-submit")
    page.wait_for_selector("#view-main", state="visible", timeout=10_000)
    _screenshot(page, "main_after_setup.png", screenshot_dir)


def test_login_flow(page: Page, base_url: str, screenshot_dir: Path) -> None:
    page.goto(base_url)
    page.wait_for_selector("#view-auth", state="visible", timeout=5_000)

    expect(page.locator("#auth-subtitle")).to_contain_text("Enter your admin password")
    expect(page.locator("#auth-submit")).to_have_text("Login")
    _screenshot(page, "login.png", screenshot_dir)

    page.fill("#auth-password", TEST_PASSWORD)
    page.click("#auth-submit")
    page.wait_for_selector("#view-main", state="visible", timeout=10_000)
    _screenshot(page, "main_after_login.png", screenshot_dir)


def test_local_terminal_renders(page: Page, base_url: str, screenshot_dir: Path) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    expect(page.locator("#terminal-container .terminal-text")).to_be_visible()
    _assert_no_garbled_text(page)
    _screenshot(page, "terminal_prompt.png", screenshot_dir)


def test_terminal_echo(page: Page, base_url: str, screenshot_dir: Path) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    _type_in_terminal(page, "echo PLAYWRIGHT_MARKER\n")
    _wait_for_marker(page, "PLAYWRIGHT_MARKER")
    _assert_no_garbled_text(page)
    _screenshot(page, "terminal_echo.png", screenshot_dir)


def test_terminal_ime_composition_commit(
    page: Page, base_url: str, screenshot_dir: Path
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    marker = "PLAYWRIGHT_IME_MARKER"
    _compose_in_terminal(page, f"echo {marker}")
    _type_in_terminal(page, "\n")
    _wait_for_marker(page, marker)
    _assert_no_garbled_text(page)
    _screenshot(page, "terminal_ime_composition.png", screenshot_dir)


def test_terminal_resize_on_connect(
    page: Page, base_url: str, screenshot_dir: Path
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    expected_cols = 132
    expected_rows = 44
    _send_terminal_resize(page, cols=expected_cols, rows=expected_rows)

    _type_in_terminal(page, "stty size\n")

    expected = f"{expected_rows} {expected_cols}"
    _wait_for_marker(page, expected)
    _screenshot(page, "terminal_resize.png", screenshot_dir)


def test_terminal_long_output_scrolls_within_fixed_container(
    page: Page, base_url: str, screenshot_dir: Path
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    marker = "PLAYWRIGHT_SCROLL_DENSE_DONE"
    _send_terminal_raw(
        page,
        f"for i in $(seq 1 320); do echo PLAYWRIGHT_SCROLL_LINE_$i; done; echo {marker}\n",
    )
    _wait_for_marker(page, marker)
    _wait_terminal_near_bottom(page)

    metrics = _terminal_scroll_metrics(page)
    assert metrics["scrollHeight"] > metrics["clientHeight"] + 200
    assert metrics["distanceFromBottom"] <= 40
    _screenshot(page, "terminal_scroll_long_output.png", screenshot_dir)


def test_terminal_manual_scroll_pauses_then_resumes_autofollow(
    page: Page, base_url: str, screenshot_dir: Path
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    initial_marker = "PLAYWRIGHT_SCROLL_MANUAL_READY"
    _send_terminal_raw(
        page,
        f"for i in $(seq 1 220); do echo PLAYWRIGHT_SCROLL_PREP_$i; done; echo {initial_marker}\n",
    )
    _wait_for_marker(page, initial_marker)
    _wait_terminal_near_bottom(page)
    _ensure_terminal_overflow(page, min_extra_px=300)

    _scroll_terminal_to_top(page)
    top_metrics = _terminal_scroll_metrics(page)
    assert top_metrics["distanceFromBottom"] > top_metrics["clientHeight"] // 2

    hold_marker = "PLAYWRIGHT_SCROLL_HOLD_MARKER"
    _send_terminal_raw(page, f"echo {hold_marker}\n")
    _wait_for_marker(page, hold_marker)
    hold_metrics = _terminal_scroll_metrics(page)
    assert hold_metrics["distanceFromBottom"] > hold_metrics["clientHeight"] // 2
    _screenshot(page, "terminal_scroll_manual_hold.png", screenshot_dir)

    _scroll_terminal_to_bottom(page)
    _wait_terminal_near_bottom(page)

    resume_marker = "PLAYWRIGHT_SCROLL_RESUME_MARKER"
    _send_terminal_raw(page, f"echo {resume_marker}\n")
    _wait_for_marker(page, resume_marker)
    _wait_terminal_near_bottom(page)
    resumed_metrics = _terminal_scroll_metrics(page)
    assert resumed_metrics["distanceFromBottom"] <= 40
    _screenshot(page, "terminal_scroll_manual_resume.png", screenshot_dir)


def test_launch_claude(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    page.select_option("#assistant-select", "claude")
    page.click("#btn-launch-ai")
    _wait_for_marker(page, "MOCK_CLAUDE_READY")
    _type_in_terminal(page, "playwright interactive check\n")
    _wait_for_marker(page, "MOCK_CLAUDE_INPUT:playwright interactive check")
    _assert_no_garbled_text(page)
    _screenshot(page, "launch_claude.png", screenshot_dir)


def test_launch_codex(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    page.select_option("#assistant-select", "codex")
    page.click("#btn-launch-ai")
    _wait_for_marker(page, "MOCK_CODEX_OK")
    _type_in_terminal(page, "echo PLAYWRIGHT_CODEX_INPUT_OK\n")
    _wait_for_marker(page, "PLAYWRIGHT_CODEX_INPUT_OK")
    _assert_no_garbled_text(page)
    _screenshot(page, "launch_codex.png", screenshot_dir)


def test_canvas_artifact_display(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)
    page.wait_for_timeout(1_000)

    daemon_url = f"http://127.0.0.1:{server_info['daemon_port']}/mcp"

    def _mcp_call(msg_id: int, method: str, params: dict) -> None:
        body = json.dumps(
            {"jsonrpc": "2.0", "id": msg_id, "method": method, "params": params}
        ).encode()
        req = urllib.request.Request(
            daemon_url,
            method="POST",
            headers={"Content-Type": "application/json"},
            data=body,
        )
        urllib.request.urlopen(req, timeout=5)

    _mcp_call(
        1,
        "initialize",
        {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "e2e-test"},
        },
    )
    _mcp_call(
        2,
        "tools/call",
        {
            "name": "canvas_render_text",
            "arguments": {
                "session_id": "e2e-test",
                "title": "E2E Test Artifact",
                "markdown_or_text": "# Hello Playwright\n\nThis is a test artifact.",
            },
        },
    )

    page.wait_for_selector("#canvas-text", state="visible", timeout=10_000)
    page.wait_for_function(
        """() => {
            const el = document.getElementById('canvas-text');
            return el && el.textContent.includes('Hello Playwright');
        }""",
        timeout=10_000,
    )
    expect(page.locator("#canvas-mode")).to_have_text("review")
    _screenshot(page, "canvas_artifact.png", screenshot_dir)


def test_canvas_image_artifact_display(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    daemon_url = f"http://127.0.0.1:{server_info['daemon_port']}/mcp"
    image_path = Path(server_info["project_dir"]) / "e2e-image.png"
    _write_test_png(image_path)

    _mcp_tool_call(
        daemon_url,
        11,
        "canvas_render_image",
        {"session_id": "local", "title": "E2E Image Artifact", "path": str(image_path)},
    )

    page.wait_for_selector("#canvas-image", state="visible", timeout=10_000)
    page.wait_for_function(
        """() => {
            const img = document.getElementById('canvas-img');
            if (!img) return false;
            if (!img.src.includes('/api/files/')) return false;
            return true;
        }""",
        timeout=10_000,
    )
    file_probe = page.evaluate(
        """() => {
            const img = document.getElementById('canvas-img');
            if (!img) return { ok: false, status: 0 };
            return fetch(img.src, { method: 'GET' })
                .then((resp) => ({ ok: resp.ok, status: resp.status }));
        }"""
    )
    assert file_probe["ok"] is True and file_probe["status"] == 200
    expect(page.locator("#canvas-mode")).to_have_text("review")
    _screenshot(page, "canvas_image_artifact.png", screenshot_dir)


def test_canvas_pdf_artifact_display(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    daemon_url = f"http://127.0.0.1:{server_info['daemon_port']}/mcp"
    pdf_path = Path(server_info["project_dir"]) / "e2e-doc.pdf"
    _write_test_pdf(pdf_path)

    _mcp_tool_call(
        daemon_url,
        21,
        "canvas_render_pdf",
        {"session_id": "local", "title": "E2E PDF Artifact", "path": str(pdf_path), "page": 0},
    )

    page.wait_for_selector("#canvas-pdf", state="visible", timeout=10_000)
    page.wait_for_function(
        """() => {
            const iframe = document.querySelector('#canvas-pdf iframe');
            return iframe && iframe.src.includes('/api/files/');
        }""",
        timeout=10_000,
    )
    expect(page.locator("#canvas-mode")).to_have_text("review")
    _screenshot(page, "canvas_pdf_artifact.png", screenshot_dir)


def test_canvas_clear_returns_prompt_mode(
    page: Page, base_url: str, screenshot_dir: Path, server_info: dict
) -> None:
    _login(page, base_url)
    _wait_terminal_ready(page)

    daemon_url = f"http://127.0.0.1:{server_info['daemon_port']}/mcp"
    _mcp_tool_call(
        daemon_url,
        31,
        "canvas_render_text",
        {
            "session_id": "local",
            "title": "To Clear",
            "markdown_or_text": "Canvas should return to prompt.",
        },
    )
    page.wait_for_selector("#canvas-text", state="visible", timeout=10_000)

    _mcp_tool_call(
        daemon_url,
        32,
        "canvas_clear",
        {"session_id": "local", "reason": "e2e-reset"},
    )
    page.wait_for_selector("#canvas-empty", state="visible", timeout=10_000)
    expect(page.locator("#canvas-mode")).to_have_text("prompt")
    _screenshot(page, "canvas_clear_prompt.png", screenshot_dir)


def test_mobile_viewport(page: Page, base_url: str, screenshot_dir: Path) -> None:
    page.set_viewport_size({"width": 375, "height": 812})
    _login(page, base_url)
    _wait_terminal_ready(page)

    direction = page.evaluate(
        "() => getComputedStyle(document.getElementById('workspace')).flexDirection"
    )
    assert direction == "column", f"expected column layout, got {direction}"

    vertical_order = page.evaluate(
        """() => {
            const canvasTop = document.getElementById('panel-canvas').getBoundingClientRect().top;
            const terminalTop = document.getElementById('panel-terminal').getBoundingClientRect().top;
            return { canvasTop, terminalTop };
        }"""
    )
    assert vertical_order["canvasTop"] < vertical_order["terminalTop"], (
        f"expected canvas above terminal in mobile layout, got {vertical_order}"
    )

    expect(page.locator("#mobile-keybar")).to_be_visible()
    expect(page.locator("#btn-terminal-minimize")).to_be_visible()

    page.click("#btn-terminal-minimize")
    expect(page.locator("#panel-terminal")).to_be_hidden()
    expect(page.locator("#terminal-pop-row")).to_be_visible()
    page.click("#btn-terminal-pop")
    expect(page.locator("#panel-terminal")).to_be_visible()
    expect(page.locator("#terminal-pop-row")).to_be_hidden()

    _screenshot(page, "mobile_layout.png", screenshot_dir)


def test_logout_clears_session(
    page: Page, base_url: str, screenshot_dir: Path
) -> None:
    _login(page, base_url)
    page.click("#btn-logout")
    page.wait_for_selector("#view-auth", state="visible", timeout=5_000)
    _screenshot(page, "after_logout.png", screenshot_dir)
