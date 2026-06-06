// Purpose: Tauri v2 application entry point for SIN-Code Desktop
// Docs: main.doc.md

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::sync::Mutex;
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    Manager, Runtime, WebviewWindowBuilder,
};
use tauri_plugin_store::StoreExt;

// ── App State ──────────────────────────────────────────────────────────────
#[derive(Default)]
struct AppState {
    config: Mutex<Option<serde_json::Value>>,
}

// ── Commands ──────────────────────────────────────────────────────────────

#[tauri::command]
async fn greet(name: &str) -> Result<String, String> {
    Ok(format!("Hello, {}! Welcome to SIN-Code Desktop.", name))
}

#[tauri::command]
async fn get_version() -> Result<String, String> {
    Ok(env!("CARGO_PKG_VERSION").to_string())
}

#[tauri::command]
async fn check_sin_cli() -> Result<bool, String> {
    use std::process::Command;
    let output = Command::new("sin")
        .arg("--version")
        .output()
        .map_err(|e| format!("Failed to run sin: {}", e))?;
    Ok(output.status.success())
}

#[tauri::command]
async fn run_sin_command(
    app: tauri::AppHandle,
    args: Vec<String>,
) -> Result<String, String> {
    use std::process::Command;
    let mut cmd = Command::new("sin");
    cmd.args(&args);
    let output = cmd
        .output()
        .map_err(|e| format!("Failed to run sin: {}", e))?;
    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).to_string();
    if output.status.success() {
        Ok(stdout)
    } else {
        Err(format!("sin {} failed: {}", args.join(" "), stderr))
    }
}

// ── Setup ──────────────────────────────────────────────────────────────────

fn setup_tray<R: Runtime>(app: &tauri::AppHandle<R>) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "Show SIN-Code", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &quit])?;

    TrayIconBuilder::new()
        .icon(tauri::image::Image::from_bytes(include_bytes!("../icons/tray-icon.png"))?)
        .menu(&menu)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
            "quit" => {
                app.exit(0);
            }
            _ => {}
        })
        .build(app)?;

    Ok(())
}

// ── Main ───────────────────────────────────────────────────────────────────

fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    tauri::Builder::default()
        .plugin(tauri_plugin_store::Builder::new().build())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_os::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_webview_window::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![
            greet,
            get_version,
            check_sin_cli,
            run_sin_command
        ])
        .setup(|app| {
            // Create the main window
            let window = WebviewWindowBuilder::new(
                app,
                "main",
                tauri::WebviewUrl::App("index.html".into()),
            )
            .title("SIN-Code Desktop")
            .inner_size(1280.0, 800.0)
            .min_inner_size(900.0, 600.0)
            .center()
            .resizable(true)
            .decorations(true)
            .transparent(false)
            .visible(true)
            .devtools(cfg!(debug_assertions))
            .build()?;

            // Setup tray
            setup_tray(app)?;

            // Load config from store
            let store = app.store("config.json")?;
            if let Some(config) = store.get("app_config") {
                *app.state::<AppState>().config.lock().unwrap() = Some(config.clone());
            }

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}