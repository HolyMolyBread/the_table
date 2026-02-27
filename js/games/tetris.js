  // ── 다인용 테트리스 (Shared Tetris) ────────────────────────────────────────

  const TETRIS_ROWS = 20;
  const TETRIS_COLS = 15;
  const TETRIS_COLORS = ['', '#ef4444', '#22c55e', '#3b82f6', '#eab308'];

  const TETRIS_SHAPES = [
    [[-1,0],[0,0],[1,0],[2,0]], [[0,-1],[0,0],[0,1],[0,2]], [[-1,0],[0,0],[1,0],[2,0]], [[0,-1],[0,0],[0,1],[0,2]],
    [[0,0],[1,0],[0,1],[1,1]], [[0,0],[1,0],[0,1],[1,1]], [[0,0],[1,0],[0,1],[1,1]], [[0,0],[1,0],[0,1],[1,1]],
    [[0,0],[-1,1],[0,1],[1,1]], [[0,0],[0,-1],[0,1],[1,0]], [[-1,0],[0,0],[1,0],[0,1]], [[0,0],[-1,0],[0,-1],[0,1]],
    [[0,0],[1,0],[-1,1],[0,1]], [[0,0],[0,-1],[1,0],[1,1]], [[0,0],[1,0],[-1,1],[0,1]], [[0,0],[1,0],[0,-1],[1,-1]],
    [[-1,0],[0,0],[0,1],[1,1]], [[0,0],[1,0],[0,-1],[1,-1]], [[-1,0],[0,0],[0,1],[1,1]], [[0,0],[1,0],[0,-1],[1,-1]],
    [[-1,0],[-1,1],[0,1],[1,1]], [[0,0],[1,0],[0,-1],[0,1]], [[-1,0],[0,0],[1,0],[1,1]], [[0,0],[-1,0],[0,-1],[0,1]],
    [[1,0],[-1,1],[0,1],[1,1]], [[0,0],[0,-1],[0,1],[1,-1]], [[-1,0],[0,0],[1,0],[-1,1]], [[0,0],[-1,0],[0,-1],[0,1]],
  ];

  let tetrisBoard = [];
  let tetrisPiece = null;
  let tetrisTurnId = '';
  let tetrisMySlot = -1;

  function showTetrisUI() {
    switchGameView('tetris');
  }

  function renderTetris(data) {
    if (!data) return;
    tetrisBoard = (data.board || []).map(r => [...r]);
    tetrisPiece = data.currentPiece ? { ...data.currentPiece } : null;
    tetrisTurnId = data.currentTurn || '';

    const players = data.players || [];
    tetrisMySlot = players.indexOf(currentUserId);

    const statusEl = document.getElementById('tetris-status');
    if (statusEl) {
      const isMyTurn = tetrisTurnId === currentUserId;
      statusEl.textContent = isMyTurn ? '🎮 내 차례! 방향키/WASD로 조작' : (tetrisTurnId ? `${tetrisTurnId}의 차례...` : '대기 중');
    }

    const scoresEl = document.getElementById('tetris-scores');
    if (scoresEl && data.scores) {
      scoresEl.innerHTML = (data.players || []).map((p, i) =>
        p ? `<span class="tetris-score" style="color:${TETRIS_COLORS[i+1]||'#999'}">${escapeHTML(p)}: ${data.scores[i]||0}</span>` : ''
      ).filter(Boolean).join(' | ');
    }

    const boardEl = document.getElementById('tetris-board');
    if (!boardEl) return;

    if (!boardEl.dataset.ready) {
      boardEl.dataset.ready = '1';
      boardEl.innerHTML = '';
      for (let r = 0; r < TETRIS_ROWS; r++) {
        for (let c = 0; c < TETRIS_COLS; c++) {
          const cell = document.createElement('div');
          cell.className = 'tetris-cell';
          cell.dataset.r = r;
          cell.dataset.c = c;
          boardEl.appendChild(cell);
        }
      }
    }

    const cells = boardEl.querySelectorAll('.tetris-cell');
    cells.forEach(cell => {
      const r = +cell.dataset.r, c = +cell.dataset.c;
      let color = tetrisBoard[r] && tetrisBoard[r][c] ? TETRIS_COLORS[tetrisBoard[r][c]] : '';
      if (tetrisPiece) {
        const turnSlot = players.indexOf(tetrisTurnId);
        const pc = turnSlot >= 0 ? turnSlot + 1 : 1;
        const shape = TETRIS_SHAPES[tetrisPiece.type * 4 + (tetrisPiece.rotation % 4)];
        for (const off of shape) {
          const cc = tetrisPiece.x + off[0];
          const rr = tetrisPiece.y + off[1];
          if (rr === r && cc === c) {
            color = TETRIS_COLORS[pc] || '#888';
            break;
          }
        }
      }
      cell.style.backgroundColor = color || 'transparent';
      cell.classList.toggle('tetris-filled', !!color);
    });

    if (data.lastClear && data.lastClear.length > 0) {
      if (window.SoundManager) {
        window.SoundManager.playPianoNote(523.25, 0.15);
        window.SoundManager.playPianoNote(659.25, 0.12);
        window.SoundManager.playPianoNote(783.99, 0.1);
      }
      const flashEl = document.getElementById('tetris-clear-flash');
      if (flashEl) {
        flashEl.textContent = data.lastScorer ? `${data.lastScorer} +${data.lastClear.length * 100}점!` : '줄 제거!';
        flashEl.classList.add('tetris-flash-visible');
        setTimeout(() => flashEl.classList.remove('tetris-flash-visible'), 1200);
      }
    }
  }

  function tetrisSendMove(dir) {
    if (tetrisTurnId !== currentUserId) return;
    if (typeof sendGameAction === 'function') {
      sendGameAction({ cmd: 'move', dir });
    }
    if (dir === 'down' && window.SoundManager) {
      window.SoundManager.playPianoNote(130.81, 0.06);
    }
  }

  function tetrisSendFlick() {
    if (tetrisTurnId !== currentUserId) return;
    if (typeof sendGameAction === 'function') {
      sendGameAction({ cmd: 'flick' });
    }
    if (window.SoundManager) window.SoundManager.playPianoNote(261.63, 0.08);
  }

  document.addEventListener('keydown', function(e) {
    if (!document.getElementById('tetris-container')?.offsetParent) return;
    if (['ArrowLeft','ArrowRight','ArrowDown','ArrowUp','KeyA','KeyD','KeyS','KeyW','Space'].includes(e.code)) {
      e.preventDefault();
    }
    switch (e.code) {
      case 'ArrowLeft': case 'KeyA': tetrisSendMove('left'); break;
      case 'ArrowRight': case 'KeyD': tetrisSendMove('right'); break;
      case 'ArrowDown': case 'KeyS': tetrisSendMove('down'); break;
      case 'ArrowUp': case 'KeyW': tetrisSendMove('rotate'); break;
      case 'Space': tetrisSendFlick(); break;
    }
  });

  window.showTetrisUI = showTetrisUI;
  window.renderTetris = renderTetris;
