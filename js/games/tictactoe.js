  // ── Tictactoe UI ───────────────────────────────────────────────────────────

  let tttMyColor = 0;    // 1=O, 2=X (0=미정)
  let tttTurnId  = '';   // 현재 차례인 유저 ID
  let tttBoardReady    = false;
  let tttPrevBoard     = null;  // diffing용 이전 보드 (3x3)

  function showTicTacToeUI() {
    switchGameView('tictactoe');
  }

  function renderTicTacToe(data) {
    if (!data) return;
    tttTurnId = data.turn || '';

    // 내 색 확인
    if (data.colors && data.colors[currentUserId]) {
      tttMyColor = data.colors[currentUserId];
    }

    // 상태 바 텍스트
    const statusEl = document.getElementById('ttt-status');
    if (statusEl) {
      if (tttTurnId === currentUserId) {
        const sym = tttMyColor === 1 ? '⭕' : '❌';
        statusEl.textContent = `${sym} 내 차례입니다!`;
        statusEl.style.color = 'var(--accent)';
      } else if (tttTurnId) {
        statusEl.innerHTML = `⏳ <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(tttTurnId)}')" title="전적 보기">${escapeHTML(tttTurnId)}</span>의 차례...`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // 색상 정보
    const infoEl = document.getElementById('ttt-color-info');
    if (infoEl && data.colors) {
      infoEl.innerHTML = Object.entries(data.colors)
        .map(([uid, col]) => `${col === 1 ? '⭕ O' : '❌ X'}: <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="전적 보기">${escapeHTML(uid)}</span>`)
        .join('  |  ');
    }

    // 3×3 보드 렌더링 (그리드 유지 + diffing)
    const boardEl = document.getElementById('ttt-board');
    if (!boardEl || !data.board) return;
    const board = data.board;
    const prev = tttPrevBoard;
    const isMyTurn = tttTurnId === currentUserId;

    if (!tttBoardReady) {
      tttBoardReady = true;
      boardEl.innerHTML = '';
      for (let r = 0; r < 3; r++) {
        for (let c = 0; c < 3; c++) {
          const cell = document.createElement('div');
          cell.className = 'ttt-cell';
          cell.dataset.r = r;
          cell.dataset.c = c;
          boardEl.appendChild(cell);
        }
      }
    }

    const cells = boardEl.querySelectorAll('.ttt-cell');
    const isBoardEmpty = board.every(row => row.every(v => v === 0));
    if (isBoardEmpty && prev && !prev.every(row => row.every(v => v === 0))) {
      boardEl.classList.add('ttt-round-reset');
      setTimeout(() => boardEl.classList.remove('ttt-round-reset'), 400);
    }
    cells.forEach(cell => {
      const r = +cell.dataset.r, c = +cell.dataset.c;
      const val = board[r][c];
      const wasEmpty = prev && prev[r][c] === 0;
      const isNew = wasEmpty && (val === 1 || val === 2);
      // 보드 초기화(무승부 등) 시 val===0인 셀에서 이전 돌이 남지 않도록 매번 완전히 비움
      cell.classList.remove('ttt-o', 'ttt-x', 'ttt-can-place');
      while (cell.firstChild) cell.removeChild(cell.firstChild);
      cell.onclick = null;
      if (val === 1) {
        cell.classList.add('ttt-o');
        const inner = document.createElement('span');
        inner.className = 'ttt-cell-inner' + (isNew ? ' animate-pop' : '');
        inner.textContent = '⭕';
        cell.appendChild(inner);
      } else if (val === 2) {
        cell.classList.add('ttt-x');
        const inner = document.createElement('span');
        inner.className = 'ttt-cell-inner' + (isNew ? ' animate-pop' : '');
        inner.textContent = '❌';
        cell.appendChild(inner);
      } else if (isMyTurn) {
        cell.classList.add('ttt-can-place');
      }
      cell.onclick = (val === 0 && isMyTurn) ? () => onTttCellClick(r, c) : null;
    });
    tttPrevBoard = board.map(row => [...row]);
  }

  function onTttCellClick(r, c) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (tttTurnId !== currentUserId) return;
    sendGameAction({ cmd: 'place', r, c });
  }