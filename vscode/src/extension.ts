import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { LanguageClientOptions } from 'vscode-languageclient';
import { LanguageClient, ServerOptions } from 'vscode-languageclient/node';

let client: LanguageClient;

function getServerCommand(): string {
	const config = vscode.workspace.getConfiguration('kvx');
	let serverPath = config.get<string>('serverPath')?.trim();

	if (serverPath) {
		if (vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders.length > 0) {
			const workspaceFolder = vscode.workspace.workspaceFolders[0].uri.fsPath;
			serverPath = serverPath.replace(/\${workspaceFolder}/g, workspaceFolder);
		}
		return serverPath;
	}

	if (process.env.KVX_SERVER_PATH) {
		return process.env.KVX_SERVER_PATH;
	}

	return '';
}

// This method is called when your extension is activated
// Your extension is activated the very first time the command is executed
export function activate(context: vscode.ExtensionContext) {

	// Use the console to output diagnostic information (console.log) and errors (console.error)
	// This line of code will only be executed once when your extension is activated
	console.log('Congratulations, your extension "kvx" is now active!');

	// The command has been defined in the package.json file
	// Now provide the implementation of the command with registerCommand
	// The commandId parameter must match the command field in package.json
	const disposable = vscode.commands.registerCommand('kvx.helloWorld', () => {
		// The code you place here will be executed every time your command is executed
		// Display a message box to the user
		vscode.window.showInformationMessage('Hello World from kvx!');
	});

	context.subscriptions.push(disposable);

	const config = vscode.workspace.getConfiguration('kvx');
	const command = getServerCommand();
	if (!command) {
		vscode.window.showErrorMessage('KVX Language Server command is not configured (settings or env.KVX_SERVER_PATH).');
		return;
	}
	const args = config.get<string[]>('serverArgs', ['-']);

	const serverOptions: ServerOptions = { command, args };

	const clientOptions: LanguageClientOptions = {
        documentSelector: [
        { scheme: "file", language: "verilog" },
        { scheme: "file", language: "systemverilog" }
        ]
    };
	client = new LanguageClient(
        "Phylotypes Language Server",
        "Language Server Extension for Verilog",
        serverOptions,
        clientOptions
    );
    client.start();
}

// This method is called when your extension is deactivated
export function deactivate() {
	if (client) {
		return client.stop();
	}
}
