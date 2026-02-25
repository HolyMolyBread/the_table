/**
 * 오목(Gomoku) 게임 전용 로직
 * - 전역 변수, 보드 렌더링, 클릭 이벤트, 액션 전송
 * - index.html의 sendGameAction, switchGameView, currentUserId, escapeForJsAttr, escapeHTML 등에 의존
 */
(function() {
    'use strict';
  
    // Gomoku 전역 변수 (index.html과 공유)
    window.gomokuMyColor    = 0;
    window.gomokuTurnUserId = '';
    window.gomokuColorMap   = {};
    window.gomokuBoardReady = false;
    window.gomokuEnded      = false; // 게임 종료 후 리매치 버튼 표시 여부
    window.gomokuPrevBoard  = null;  // diffing용 이전 보드 (15x15)
  
    const STAR_POINTS = new Set(
      [3,7,11].flatMap(r => [3,7,11].map(c => `${r},${c}`))
    );
  
    /** 오목 보드 표시 및 초기화 */
    window.showGomokuBoard = function showGomokuBoard() {
      switchGameView('omok');
      if (!window.gomokuBoardReady) { createGomokuBoard(); window.gomokuBoardReady = true; }
      scaleBoardToFit();
    };
  
    /** 모바일에서 오목 보드가 화면 너비를 넘지 않도록 transform: scale() 적용 */
    function scaleBoardToFit() {
      const board   = document.getElementById('gomoku-board');
      const scaler  = document.getElementById('gomoku-board-scaler');
      if (!board || !scaler) return;
      const boardW  = 490; // 보드 실제 크기 + 여백 고려
      const available = window.innerWidth - 20;
      if (available < boardW) {
        const ratio = available / boardW;
        board.style.transform       = `scale(${ratio})`;
        board.style.transformOrigin = 'top center';
        // scaler 높이를 축소된 보드 높이에 맞춤 (레이아웃 붕괴 방지)
        board.style.marginBottom    = `-${Math.round(boardW * (1 - ratio))}px`;
      } else {
        board.style.transform    = '';
        board.style.marginBottom = '';
      }
    }
    window.addEventListener('resize', scaleBoardToFit);
  
    function createGomokuBoard() {
      const el = document.getElementById('gomoku-board');
      el.innerHTML = '';
      for (let r = 0; r < 15; r++) {
        for (let c = 0; c < 15; c++) {
          const cell = document.createElement('div');
          cell.className = 'gomoku-cell';
          cell.dataset.r = r; cell.dataset.c = c;
          if (r === 0)  cell.classList.add('edge-top');
          if (r === 14) cell.classList.add('edge-bottom');
          if (c === 0)  cell.classList.add('edge-left');
          if (c === 14) cell.classList.add('edge-right');
          if (STAR_POINTS.has(`${r},${c}`)) cell.classList.add('star-point');
          cell.addEventListener('click', () => onGomokuCellClick(r, c));
          el.appendChild(cell);
        }
      }
    }
  
    /** 오목 보드 렌더링 (board_update, game_result 이벤트에서 호출) */
    window.renderBoard = function renderBoard(data) {
      if (!window.gomokuBoardReady) return;
      const board = data.board;
      const prev = window.gomokuPrevBoard;
      const cells = document.querySelectorAll('.gomoku-cell');
      cells.forEach(cell => {
        const r = +cell.dataset.r, c = +cell.dataset.c;
        const stone = board[r][c];
        const wasEmpty = prev && prev[r][c] === 0;
        const isNew = wasEmpty && (stone === 1 || stone === 2);
        cell.querySelector('.gomoku-stone')?.remove();
        cell.classList.remove('can-place');
        if (stone === 1 || stone === 2) {
          const s = document.createElement('div');
          s.className = `gomoku-stone ${stone === 1 ? 'black' : 'white'}${isNew ? ' animate-pop' : ''}`;
          cell.appendChild(s);
        }
      });
      window.gomokuPrevBoard = board.map(row => [...row]);
      const lm = data.lastMove;
      if (lm && lm[0] >= 0) {
        document.querySelector(`.gomoku-cell[data-r="${lm[0]}"][data-c="${lm[1]}"]`)
                ?.querySelector('.gomoku-stone')?.classList.add('last-move');
      }
      if (data.colors) {
        window.gomokuColorMap = data.colors;
        if (data.colors[currentUserId]) window.gomokuMyColor = data.colors[currentUserId];
      }
      window.gomokuTurnUserId = data.turn;
      if (data.turn === currentUserId) {
        cells.forEach(cell => {
          const r = +cell.dataset.r, c = +cell.dataset.c;
          if (data.board[r][c] === 0) cell.classList.add('can-place');
        });
      }
  
      // 관전자 판별: colors 맵에 내 ID가 없으면 관전자
      if (data.colors && Object.keys(data.colors).length > 0) {
        const isSpectator = !data.colors[currentUserId];
        if (isSpectator) {
          document.getElementById('rematch-area').classList.remove('visible');
          document.getElementById('gomoku-spectator-msg').style.display = 'block';
        } else {
          document.getElementById('gomoku-spectator-msg').style.display = 'none';
          // 리매치 버튼은 game_result 이벤트가 별도로 처리
        }
      }
      updateStatusBar(data.turn);
      updateColorInfo(data.colors);
    };
  
    function updateStatusBar(turnUserId) {
      const stoneEl = document.getElementById('status-stone-icon');
      const userEl  = document.getElementById('status-turn-user');
      if (!stoneEl || !userEl) return;
      if (turnUserId) {
        const col = window.gomokuColorMap[turnUserId];
        stoneEl.textContent = col === 1 ? '⚫' : col === 2 ? '⚪' : '●';
        const isMine = turnUserId === currentUserId;
        const suffix = isMine ? ' (나)' : '';
        userEl.innerHTML = `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(turnUserId)}')" title="전적 보기">${escapeHTML(turnUserId)}</span>${suffix}`;
        userEl.style.color = isMine ? '#3fb950' : 'var(--text-primary)';
      }
    }
  
    function updateColorInfo(colors) {
      const el = document.getElementById('gomoku-color-info');
      if (!colors || !Object.keys(colors).length) return;
      el.innerHTML = Object.entries(colors)
        .map(([uid, col]) => `${col === 1 ? '⚫' : '⚪'} <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="전적 보기">${escapeHTML(uid)}</span>`)
        .join('  vs  ');
    }
  
    /** 오목 게임 종료 처리 */
    window.setGomokuEnded = function setGomokuEnded() {
      window.gomokuTurnUserId = '';
      window.gomokuEnded      = true;
      const userEl = document.getElementById('status-turn-user');
      if (userEl) userEl.textContent = '게임 종료';
      const stoneEl = document.getElementById('status-stone-icon');
      if (stoneEl) stoneEl.textContent = '🏁';
      const secsEl = document.getElementById('status-seconds');
      if (secsEl) secsEl.textContent = '--';
      document.getElementById('status-timer-block')?.classList.remove('urgent');
      document.querySelectorAll('.gomoku-cell').forEach(c => c.classList.remove('can-place'));
    };
  
    /** 오목 셀 클릭 시 액션 전송 */
    function onGomokuCellClick(r, c) {
      if (!currentRoomId || !ws || ws.readyState !== WebSocket.OPEN) return;
      if (window.gomokuTurnUserId !== currentUserId) return;
      sendGameAction({ cmd: 'place', x: r, y: c });
    }
  
  })();