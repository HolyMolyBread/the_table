  // ── 서부의 결투 (Western Duel) ───────────────────────────────────────────────

  let duelPhase = 'waiting';
  let duelDrawReceivedAt = 0;
  let duelTensionInterval = null;

  function showDuelUI() {
    switchGameView('duel');
  }

  function renderDuel(data) {
    if (!data) return;
    duelPhase = data.phase || 'waiting';
    const players = data.players || ['', ''];

    const leftEl = document.getElementById('duel-avatar-left');
    const rightEl = document.getElementById('duel-avatar-right');
    const watchEl = document.getElementById('duel-watch');
    const statusEl = document.getElementById('duel-status');

    if (leftEl) leftEl.textContent = players[0] ? players[0].charAt(0).toUpperCase() : '?';
    if (rightEl) rightEl.textContent = players[1] ? players[1].charAt(0).toUpperCase() : '?';

    if (statusEl) {
      if (duelPhase === 'waiting') statusEl.textContent = '상대를 기다리는 중...';
      else if (duelPhase === 'ready') statusEl.textContent = '준비하세요...';
      else if (duelPhase === 'draw') statusEl.textContent = '쏘세요!';
      else statusEl.textContent = '';
    }

    const wrap = document.getElementById('duel-wrap');
    if (!wrap) return;

    wrap.classList.remove('duel-sepia', 'duel-flash');
    if (duelPhase === 'ready') {
      wrap.classList.add('duel-sepia');
      startDuelTensionSound();
    } else if (duelPhase === 'draw') {
      wrap.classList.remove('duel-sepia');
      wrap.classList.add('duel-flash');
      stopDuelTensionSound();
      if (window.SoundManager) window.SoundManager.playPianoNote(1975.53, 0.2);
    } else {
      stopDuelTensionSound();
    }
  }

  function startDuelTensionSound() {
    stopDuelTensionSound();
    if (!window.SoundManager) return;
    let count = 0;
    duelTensionInterval = setInterval(() => {
      window.SoundManager.playPianoNote(55 + (count % 3) * 11, 0.08);
      count++;
    }, 400);
  }

  function stopDuelTensionSound() {
    if (duelTensionInterval) {
      clearInterval(duelTensionInterval);
      duelTensionInterval = null;
    }
  }

  function handleDuelDraw(data) {
    duelDrawReceivedAt = performance.now();
    duelPhase = 'draw';
    const wrap = document.getElementById('duel-wrap');
    if (wrap) {
      wrap.classList.remove('duel-sepia');
      wrap.classList.add('duel-flash');
    }
    stopDuelTensionSound();
    if (window.SoundManager) window.SoundManager.playPianoNote(1975.53, 0.2);
    const statusEl = document.getElementById('duel-status');
    if (statusEl) statusEl.textContent = '쏘세요!';
  }

  function handleDuelClick() {
    if (duelPhase !== 'ready' && duelPhase !== 'draw') return;
    let ms = 0;
    if (duelPhase === 'draw') {
      ms = Math.round(performance.now() - duelDrawReceivedAt);
      if (ms < 0) ms = 0;
      if (window.SoundManager) window.SoundManager.playPianoNote(2093, 0.08);
    }
    if (typeof sendGameAction === 'function') {
      sendGameAction({ cmd: 'shoot', ms });
    }
    duelPhase = 'result';
  }

  window.showDuelUI = showDuelUI;
  window.renderDuel = renderDuel;
  window.handleDuelDraw = handleDuelDraw;
  window.handleDuelClick = handleDuelClick;
  window.clearDuel = function() {
    stopDuelTensionSound();
    duelPhase = 'waiting';
    duelDrawReceivedAt = 0;
    const wrap = document.getElementById('duel-wrap');
    if (wrap) wrap.classList.remove('duel-sepia', 'duel-flash');
  };
