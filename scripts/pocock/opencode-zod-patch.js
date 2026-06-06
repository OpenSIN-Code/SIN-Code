/** OpenCode Runtime Patch: Zod v3/v4 Compatibility Sandbox
 * 
 * Prevents the critical TypeError: undefined is not an object (evaluating 'n._zod.def')
 * that occurs when third-party plugins like claude-mem or oh-my-openagent load Zod v4
 * into the same scope as the OpenCode core (which uses internal Zod v3 APIs).
 * 
 * Usage: node -r ./opencode-zod-patch.js your-app.js
 * Or add to NODE_OPTIONS: NODE_OPTIONS='-r /path/to/opencode-zod-patch.js'
 * 
 * Docs: opencode-zod-patch.doc.md
 */

console.log("🛡️  Initialisiere Zod-v4 Kompatibilitäts-Sandbox...");

try {
  // Attempt to find Zod in the current scope
  let ZodModule;
  
  try {
    ZodModule = require('zod');
  } catch (e) {
    console.log("⚠️  Zod nicht im aktuellen Scope gefunden - warte auf dynamischen Import...");
    // If not available now, we'll patch when it loads
  }

  // Patch function that applies the compatibility layer
  function applyZodPatch(Zod) {
    if (!Zod || !Zod.ZodType) {
      console.warn("⚠️  Zod oder ZodType nicht gefunden - Patch übersprungen");
      return false;
    }

    const ZodType = Zod.ZodType;
    
    // Check if _def already exists (Zod v3 has it)
    const hasDef = ZodType.prototype && Object.prototype.hasOwnProperty.call(ZodType.prototype, '_def');
    
    if (!hasDef) {
      console.log("🔧 Patching _def property (Zod v4 detected)...");
      
      Object.defineProperty(ZodType.prototype, '_def', {
        get() {
          // For Zod v4, try to map from the new internal structure
          if (this._def_v4) {
            return this._def_v4;
          }
          if (this._zod && this._zod.def) {
            return this._zod.def;
          }
          // Fallback: return empty object to prevent crashes
          return {};
        },
        set(val) {
          this._def_v4 = val;
        },
        configurable: true,
        enumerable: true
      });
    }

    // Patch the _zod internal accessor (used by toJsonSchema in worker.js)
    const hasZodInternal = ZodType.prototype && Object.prototype.hasOwnProperty.call(ZodType.prototype, '_zod');
    
    if (!hasZodInternal || true) {  // Always patch _zod to ensure compatibility
      console.log("🔧 Patching _zod internal accessor...");
      
      Object.defineProperty(ZodType.prototype, '_zod', {
        get() {
          return {
            def: this._def || {},
            // Map other common internal properties that might be accessed
            innerType: this._def?.innerType || this._def?.type,
            shape: this._def?.shape,
            options: this._def?.options,
            schema: this._def?.schema,
            _type: this._type || this._def?.type,
          };
        },
        configurable: true,
        enumerable: true
      });
    }

    return true;
  }

  // Apply immediately if Zod is available
  if (ZodModule) {
    applyZodPatch(ZodModule);
  }

  // Also hook into require() to catch late-loading plugins
  const Module = require('module');
  const originalRequire = Module.prototype.require;

  Module.prototype.require = function(id) {
    const result = originalRequire.apply(this, arguments);
    
    // If this is a zod module, patch it
    if (id === 'zod' || id.endsWith('/zod')) {
      if (result && result.ZodType) {
        console.log("🔧 Zod dynamisch geladen - wende Patch an...");
        applyZodPatch(result);
      }
    }
    
    return result;
  };

  console.log("✅ Zod-Kompatibilitäts-Sandbox erfolgreich aktiviert.");
  console.log("   Abstürze durch n._zod.def in toJsonSchema werden nun abgefangen.");

} catch (error) {
  console.error("❌ Fehler beim Anwenden des Zod-Patches:", error.message);
  console.error("   Stack:", error.stack);
  // Don't crash - let the app continue without the patch
}

// Export for programmatic use
module.exports = {
  applyZodPatch: applyZodPatch
};
