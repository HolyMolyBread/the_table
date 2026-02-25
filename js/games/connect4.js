  // ── Connect 4 UI ───────────────────────────────────────────────────────────

  let c4MyColor = 0;  // 1=빨강, 2=노랑 (0=미정)
  let c4TurnId  = ''; // 현재 차례인 유저 ID
  let c4BoardReady     = false;
  let c4PrevBoard      = null;  // diffing용 이전 보드 (6x7)

  function showConnect4UI() {
    switchGameView('connect4');
  }

  function renderConnect4(data) {
    if (!data) return;
    c4TurnId = data.turn || '';

    // 내 색 확인
    if (data.colors && data.colors[currentUserId]) {
      c4MyColor = data.colors[currentUserId];
    }

    const isMyTurn = c4TurnId === currentUserId;

    // 상태 바
    const statusEl = document.getElementById('c4-status');
    if (statusEl) {
      if (isMyTurn) {
        const sym = c4MyColor === 1 ? '🔴' : '🟡';
        statusEl.textContent = `${sym} 내 차례입니다! 열을 선택하세요.`;
        statusEl.style.color = 'var(--accent)';
      } else if (c4TurnId) {
        statusEl.innerHTML = `⏳ <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(c4TurnId)}')" title="전적 보기">${escapeHTML(c4TurnId)}</span>의 차례...`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // 색상 정보
    const infoEl = document.getElementById('c4-color-info');
    if (infoEl && data.colors) {
      infoEl.innerHTML = Object.entries(data.colors)
        .map(([uid, col]) => `${col === 1 ? '🔴 빨강' : '🟡 노랑'}: <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="전적 보기">${escapeHTML(uid)}</span>`)
        .join('  |  ');
    }

    // 열 선택 버튼 렌더링
    const colBtnsEl = document.getElementById('c4-col-btns');
    if (colBtnsEl) {
      colBtnsEl.innerHTML = '';
      for (let c = 0; c < 7; c++) {
        const btn = document.createElement('button');
        btn.className = 'c4-col-btn';
        btn.textContent = '▼';
        btn.disabled = !isMyTurn;
        if (isMyTurn) {
          btn.addEventListener('click', () => onC4ColClick(c));
        }
        colBtnsEl.appendChild(btn);
      }
    }

    // 6×7 보드 렌더링 (그리드 유지 + diffing)
    const boardEl = document.getElementById('c4-board');
    if (!boardEl || !data.board) return;
    const board = data.board;
    const prev = c4PrevBoard;

    if (!c4BoardReady) {
      c4BoardReady = true;
      boardEl.innerHTML = '';
      for (let r = 0; r < 6; r++) {
        for (let c = 0; c < 7; c++) {
          const cell = document.createElement('div');
          cell.className = 'c4-cell';
          cell.dataset.r = r;
          cell.dataset.c = c;
          boardEl.appendChild(cell);
        }
      }
    }

    const cells = boardEl.querySelectorAll('.c4-cell');
    cells.forEach(cell => {
      const r = +cell.dataset.r, c = +cell.dataset.c;
      const val = board[r][c];
      const wasEmpty = prev && prev[r][c] === 0;
      const isNew = wasEmpty && (val === 1 || val === 2);
      const isLast = r === data.lastRow && c === data.lastCol;
      cell.querySelector('.c4-piece')?.remove();
      cell.classList.remove('c4-last');
      if (val === 1 || val === 2) {
        const piece = document.createElement('div');
        piece.className = `c4-piece ${val === 1 ? 'c4-red' : 'c4-yellow'}${isNew ? ' animate-drop' : ''}`;
        cell.appendChild(piece);
        if (isLast) cell.classList.add('c4-last');
      }
    });
    c4PrevBoard = board.map(row => [...row]);
  }

  function onC4ColClick(col) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (c4TurnId !== currentUserId) return;
    sendGameAction({ cmd: 'place', col });
  }