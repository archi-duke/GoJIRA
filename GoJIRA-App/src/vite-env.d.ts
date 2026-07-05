/// <reference types="vite/client" />

// 런타임 env 주입 (docs/platform-contract/05 §5)
interface Window {
  _env_?: Record<string, string>;
}
