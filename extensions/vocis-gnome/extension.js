// Vocis Hotkey Bridge — GNOME Shell extension.
//
// Registers a global accelerator via Mutter and exposes Activated/Deactivated
// D-Bus signals so the vocis daemon (running as a normal user process) can
// implement hold-to-talk dictation on GNOME Wayland, where global hotkeys
// from third-party processes are otherwise unreachable.
//
// Press detection: Mutter's accelerator-activated signal fires when the user
// presses ctrl+shift+space. Release detection: poll global modifier state
// every POLL_INTERVAL_MS until ctrl OR shift is no longer held, then emit
// Deactivated. Polling is gated to only the time between Activated and
// Deactivated, so steady-state cost is zero.
//
// D-Bus surface (well-known name `io.github.duboisf.Vocis.Hotkey`,
// path `/io/github/duboisf/Vocis/Hotkey`,
// interface `io.github.duboisf.Vocis.Hotkey`):
//   signal Activated(s shortcut)
//   signal Deactivated(s shortcut)
//   method GetShortcut() -> s
//   method GetFocusedWindow() -> (s wm_class, s title, s id)

import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import Meta from 'gi://Meta';
import Shell from 'gi://Shell';
import Clutter from 'gi://Clutter';

import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';

const ACCELERATOR = '<Ctrl><Shift>space';
const SHORTCUT_LABEL = 'ctrl+shift+space';
const POLL_INTERVAL_MS = 30;

const BUS_NAME = 'io.github.duboisf.Vocis.Hotkey';
const OBJECT_PATH = '/io/github/duboisf/Vocis/Hotkey';
const INTERFACE_NAME = 'io.github.duboisf.Vocis.Hotkey';

const INTERFACE_XML = `
<node>
  <interface name="${INTERFACE_NAME}">
    <signal name="Activated">
      <arg type="s" name="shortcut"/>
    </signal>
    <signal name="Deactivated">
      <arg type="s" name="shortcut"/>
    </signal>
    <method name="GetShortcut">
      <arg direction="out" type="s" name="shortcut"/>
    </method>
    <method name="GetFocusedWindow">
      <arg direction="out" type="s" name="wm_class"/>
      <arg direction="out" type="s" name="title"/>
      <arg direction="out" type="s" name="id"/>
    </method>
  </interface>
</node>`;

export default class VocisHotkeyExtension extends Extension {
    enable() {
        this._actionId = 0;
        this._activatedHandler = 0;
        this._pollSourceId = 0;
        this._busOwnerId = 0;
        this._dbusImpl = null;
        this._isHeld = false;

        this._exportDbus();
        this._registerAccelerator();
    }

    disable() {
        this._stopPolling();
        this._unregisterAccelerator();
        this._unexportDbus();
    }

    // -- D-Bus -------------------------------------------------------------

    _exportDbus() {
        this._dbusImpl = Gio.DBusExportedObject.wrapJSObject(INTERFACE_XML, {
            GetShortcut: () => SHORTCUT_LABEL,
            GetFocusedWindow: () => this._getFocusedWindow(),
        });
        this._dbusImpl.export(Gio.DBus.session, OBJECT_PATH);

        this._busOwnerId = Gio.bus_own_name(
            Gio.BusType.SESSION,
            BUS_NAME,
            Gio.BusNameOwnerFlags.NONE,
            null,
            null,
            () => {
                console.warn(`[vocis] failed to acquire bus name ${BUS_NAME} — another instance running?`);
            },
        );
    }

    _unexportDbus() {
        if (this._busOwnerId !== 0) {
            Gio.bus_unown_name(this._busOwnerId);
            this._busOwnerId = 0;
        }
        if (this._dbusImpl) {
            this._dbusImpl.unexport();
            this._dbusImpl = null;
        }
    }

    _emitSignal(name) {
        if (!this._dbusImpl) return;
        this._dbusImpl.emit_signal(name, GLib.Variant.new('(s)', [SHORTCUT_LABEL]));
    }

    // Returns [wm_class, title, id] for the currently focused Mutter window.
    // wm_class follows X11 WM_CLASS for XWayland windows and the Wayland
    // app_id for native Wayland clients. id is Mutter's stable uint64 window
    // ID stringified — it is NOT an X11 window ID and cannot be passed to
    // xdotool. Returns ['', '', ''] when no window has focus.
    _getFocusedWindow() {
        const win = global.display.get_focus_window();
        if (!win) {
            return ['', '', ''];
        }
        const wmClass = win.get_wm_class() || '';
        const title = win.get_title() || '';
        const id = String(win.get_id());
        return [wmClass, title, id];
    }

    // -- Accelerator -------------------------------------------------------

    _registerAccelerator() {
        const flags = Meta.KeyBindingFlags.NONE;
        const modes = Shell.ActionMode.NORMAL
            | Shell.ActionMode.OVERVIEW
            | Shell.ActionMode.POPUP;

        this._actionId = global.display.grab_accelerator(ACCELERATOR, flags);
        if (this._actionId === Meta.KeyBindingAction.NONE || this._actionId === 0) {
            console.warn(`[vocis] grab_accelerator(${ACCELERATOR}) failed — another app may own it`);
            return;
        }

        // The accelerator is registered with Mutter, but Main.wm gates which
        // shell action modes are allowed to fire it. Opt into the normal
        // foreground modes so the daemon receives the press.
        const name = Meta.external_binding_name_for_action(this._actionId);
        Main.wm.allowKeybinding(name, modes);

        this._activatedHandler = global.display.connect(
            'accelerator-activated',
            (_display, action, _deviceId, _timestamp) => {
                if (action !== this._actionId) return;
                this._onActivated();
            },
        );
    }

    _unregisterAccelerator() {
        if (this._activatedHandler !== 0) {
            global.display.disconnect(this._activatedHandler);
            this._activatedHandler = 0;
        }
        if (this._actionId !== 0) {
            global.display.ungrab_accelerator(this._actionId);
            this._actionId = 0;
        }
    }

    // -- Press / release ---------------------------------------------------

    _onActivated() {
        // Mutter may fire activations while the key is auto-repeating. We
        // dedupe so the daemon only sees one Activated until the user
        // physically releases the modifiers.
        if (this._isHeld) return;
        this._isHeld = true;
        this._emitSignal('Activated');
        this._startPolling();
    }

    _onDeactivated() {
        if (!this._isHeld) return;
        this._isHeld = false;
        this._stopPolling();
        this._emitSignal('Deactivated');
    }

    _startPolling() {
        if (this._pollSourceId !== 0) return;
        this._pollSourceId = GLib.timeout_add(
            GLib.PRIORITY_DEFAULT,
            POLL_INTERVAL_MS,
            () => this._pollOnce(),
        );
    }

    _stopPolling() {
        if (this._pollSourceId !== 0) {
            GLib.source_remove(this._pollSourceId);
            this._pollSourceId = 0;
        }
    }

    _pollOnce() {
        const [, , mods] = global.get_pointer();
        const ctrlHeld = (mods & Clutter.ModifierType.CONTROL_MASK) !== 0;
        const shiftHeld = (mods & Clutter.ModifierType.SHIFT_MASK) !== 0;
        if (ctrlHeld && shiftHeld) {
            return GLib.SOURCE_CONTINUE;
        }
        // Either modifier was released — treat the gesture as ended.
        this._pollSourceId = 0;
        this._onDeactivated();
        return GLib.SOURCE_REMOVE;
    }
}

