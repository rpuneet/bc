#!/usr/bin/env node
import { render } from 'ink';
import process from 'node:process';
import { App } from './app.js';

// Entry point for bc TUI
// Renders the main App component using Ink

// Check if stdin is a TTY - Ink requires raw mode which only works with TTY
if (!process.stdin.isTTY) {
  console.error('Error: bc home requires an interactive terminal.');
  console.error('');
  console.error('The TUI dashboard needs a terminal that supports interactive input.');
  console.error('This error occurs when:');
  console.error('  - Running in a non-interactive shell (e.g., piped input)');
  console.error('  - Running inside a script without TTY allocation');
  console.error('  - SSH without -t flag (use: ssh -t host "bc home")');
  console.error('');
  console.error('Alternatives:');
  console.error('  bc status       # View agent status (non-interactive)');
  console.error('  mycel agent list   # List agents');
  console.error('  bc notify list  # List notification sources');
  process.exit(1);
}

render(<App />);
