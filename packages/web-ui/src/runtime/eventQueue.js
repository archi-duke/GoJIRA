/**
 * 비동기 이벤트 큐 — 이벤트를 순차적으로 처리 (이전 완료 후 다음 실행)
 *
 * 사용법:
 *   const queue = createEventQueue();
 *   queue.push(async () => { await fetchTree(); updateState(); });
 *   queue.push(async () => { await anotherAsyncOp(); });
 *   // → 첫 번째 완료 후 두 번째 실행
 */
export function createEventQueue() {
  let queue = [];
  let processing = false;

  async function process() {
    if (processing) return;
    processing = true;
    while (queue.length > 0) {
      const task = queue.shift();
      try {
        await task();
      } catch (e) {
        console.error('[EventQueue] task error:', e);
      }
    }
    processing = false;
  }

  return {
    push(task) {
      queue.push(task);
      process();
    },
    clear() {
      queue = [];
    },
  };
}
