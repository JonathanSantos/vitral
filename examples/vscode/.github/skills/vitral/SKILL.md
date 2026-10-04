---
name: vitral
description: Use when the user wants something done or checked in a web page through the Vitral browser tools (open, snapshot, click, type, select, screenshot, network, mock, flow) - looking at or describing a page, reading an article, filling a form, testing a web app, simulating API errors, or recording and replaying a repetitive browser task. Explains how to work safely while a person watches the browser window, when to ask before acting, how to handle the pause and password hand-off, and how to save reusable flows.
---
<!-- vitral-skill-version: dev -->

# Working with the Vitral browser

A person is watching the browser window while you work. Act like a careful colleague using their screen.

## Before each action
- Say in one short sentence what you are about to do and why ("Vou abrir o portal de pedidos e procurar o pedido 1023.").
- Use the site the way a person would: its own search box, menus and buttons, not hand-made URLs, unless the user gave you the URL.

## Seeing the page
- `open` and every action already return what changed: read that before calling `snapshot` again.
- Elements appear as `[eN] role "name"`. Use refs from the latest response; if a ref is stale, call `snapshot`.
- Paragraphs are not in the element list. To read text use `snapshot` with `text=true` (links come as `text[eN]`, headings as `## Title[eN]`); to look for something specific use `find=<words>`; to read one section use `scope=<heading ref>`.
- Prefer `snapshot` over `screenshot`. Use `screenshot` only when layout, colors or images matter, or the user asks to see the page.
- Never invent what is not on the screen. If something is missing, say so.

## Ask first
- Never submit, send, save, publish, buy, delete or confirm anything without a clear "sim"/"yes" from the user in the chat for that specific action. Before asking, show a short summary of what will be sent.
- Never type passwords or other secrets. When you reach a password field, Vitral hands the window to the person ("Sua vez"): tell them, in their language, to type it in the browser window and click "Retomar", then call `snapshot`.
- Logins, CAPTCHAs and payment steps belong to the person: ask them to do it in the window and tell you when they are done.
- On cookie or consent banners, choose the option that rejects non-essential cookies unless the user said otherwise.

## When the person pauses
- If a tool answers that the user paused, stop. Do not retry. Tell the person to click "Retomar" in the browser when they are done, then call `snapshot` to see what they changed and continue from there.

## Repetitive tasks
- Suggest saving a flow when the person repeats a task, says it is frequent ("toda semana", "de novo", "sempre"), or asks you to automate it.
- Every action ends with `flow step #N`. Save with `flow_save` using `fromStep`/`toStep` for exactly the steps of the task, `params` for the values that change (each at least 3 characters, e.g. `{numero: "1023"}`), and `expect=<text visible at the end>` so `flow_run` checks the result.
- Before saving, call `flow_save` with `preview=true`. It lists the steps it would save (number, action, target) without saving. Show that list to the person in plain words and confirm it is exactly the task; leave out extra steps (a wrong click, a scroll to look around, a page you opened to answer a question) with `excludeSteps=[N]`. If the preview shows a warning (a step that depends on the typed value, like a search suggestion; typing without pressing Enter at the end; an `expect` that is not on the page), fix it before saving: redo the search with `type` and `submit=true` and save that step instead, or pick text that is visible on the final page. A save with warnings is refused; use `ignoreWarnings=true` only when the person confirms the flow is right as it is. Then call `flow_save` again without `preview`; it saves the steps of the last preview.
- Run with `flow_run`. If a step fails, explain in plain words which step and why, and offer to finish by hand and record the flow again.

## Simulating API problems
- Mocks only fake responses inside this browser; no real server is touched. Find the API calls with `network_list`, then use `mock_add` (status 500, `fail: timeout`, `delayMs`).
- Call `mock_clear` between scenarios and always at the end, so nothing stays simulated.

## Answering
- Reply in the user's language, in plain words. Mention refs (`e12`) only when they help.
- When something looks broken (error message, empty area, endless loading), check `console_list` and `network_list` and explain what you found.
