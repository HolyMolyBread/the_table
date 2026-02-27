  // ── 수박게임 (Suika) - 실시간 충전제 ────────────────────────────────────────────

  const SUIKA_W = 400;
  const SUIKA_H = 500;
  const SUIKA_RECHARGE_MS = 3000;
  const SUIKA_MAX_CHARGES = 2;
  const SUIKA_FRUIT_NAMES = ['체리','딸기','포도','데코폰','오렌지','사과','배','복숭아','파인애플','멜론','수박'];
  const SUIKA_FRUIT_COLORS = ['#e11d48','#dc2626','#7c3aed','#f97316','#ea580c','#22c55e','#16a34a','#ec4899','#facc15','#84cc16','#15803d'];

  let suikaMySlot = -1;
  let suikaChargedCount = 0;
  let suikaLastChargeAt = 0;
  let suikaFruits = [];
  let suikaGameStarted = false;
  let suikaChargeInterval = null;

  function showSuikaUI() {
    switchGameView('suika');
  }

  function getSuikaChargeProgress() {
    if (suikaChargedCount >= SUIKA_MAX_CHARGES) return 1;
    const now = Date.now();
    const elapsed = now - suikaLastChargeAt;
    const progress = Math.min(1, elapsed / SUIKA_RECHARGE_MS);
    return progress;
  }

  function updateSuikaRechargeBar() {
    const bar = document.getElementById('suika-recharge-bar');
    const fill = document.getElementById('suika-recharge-fill');
    const label = document.getElementById('suika-recharge-label');
    if (!bar || !fill) return;

    const progress = getSuikaChargeProgress();
    fill.style.width = (progress * 100) + '%';

    const canDrop = suikaChargedCount >= 1;
    if (label) {
      label.textContent = canDrop ? `과일 ${suikaChargedCount}/${SUIKA_MAX_CHARGES} 충전됨` : `충전 중... ${Math.ceil((1 - progress) * SUIKA_RECHARGE_MS / 1000)}초`;
    }
    bar.classList.toggle('suika-ready', canDrop);
  }

  function playSuikaChargeSound() {
    if (window.SoundManager) {
      window.SoundManager.playPianoNote(391.99, 0.1);  // 솔(G4)
      setTimeout(() => {
        if (window.SoundManager) window.SoundManager.playPianoNote(523.25, 0.12);  // 도(C5)
      }, 80);
    }
  }

  function renderSuika(data) {
    if (!data) return;

    const players = data.players || [];
    suikaMySlot = players.indexOf(currentUserId);
    suikaGameStarted = data.gameStarted || false;
    suikaFruits = (data.fruits || []).map(f => ({ ...f }));

    const charges = data.charges || [];
    const myCharge = suikaMySlot >= 0 ? charges[suikaMySlot] : null;
    const prevCount = suikaChargedCount;
    if (myCharge) {
      suikaChargedCount = Math.min(SUIKA_MAX_CHARGES, myCharge.chargedCount || 0);
      suikaLastChargeAt = myCharge.lastChargeAt || Date.now();
    }

    if (suikaChargedCount > prevCount && suikaGameStarted) {
      playSuikaChargeSound();
    }

    const statusEl = document.getElementById('suika-status');
    if (statusEl) {
      if (!suikaGameStarted) {
        statusEl.textContent = '2명 이상 Ready 시 게임 시작';
      } else {
        statusEl.textContent = suikaChargedCount >= 1 ? '과일이 충전되었습니다! 컨테이너를 클릭하여 투하' : '과일 충전 중...';
      }
    }

    updateSuikaRechargeBar();

    const container = document.getElementById('suika-container-box');
    if (!container) return;

    const dropZone = document.getElementById('suika-drop-zone');
    if (dropZone) {
      dropZone.style.pointerEvents = suikaChargedCount >= 1 ? 'auto' : 'none';
      dropZone.style.opacity = suikaChargedCount >= 1 ? '1' : '0.5';
      dropZone.onclick = handleSuikaDrop;
    }

    const fruitsEl = document.getElementById('suika-fruits');
    if (fruitsEl) {
      fruitsEl.innerHTML = '';
      suikaFruits.forEach(f => {
        const div = document.createElement('div');
        div.className = 'suika-fruit';
        div.style.left = (f.x - f.radius) + 'px';
        div.style.top = (f.y - f.radius) + 'px';
        div.style.width = (f.radius * 2) + 'px';
        div.style.height = (f.radius * 2) + 'px';
        div.style.borderRadius = '50%';
        div.style.backgroundColor = SUIKA_FRUIT_COLORS[f.type] || '#888';
        div.style.border = '2px solid rgba(255,255,255,0.4)';
        div.title = SUIKA_FRUIT_NAMES[f.type] || '과일';
        fruitsEl.appendChild(div);
      });
    }
  }

  function handleSuikaDrop(e) {
    if (suikaMySlot < 0 || suikaChargedCount < 1) return;
    const zone = document.getElementById('suika-drop-zone');
    if (!zone) return;
    const rect = zone.getBoundingClientRect();
    const scaleX = SUIKA_W / rect.width;
    const x = (e.clientX - rect.left) * scaleX;
    if (typeof sendGameAction === 'function') {
      sendGameAction({ cmd: 'drop', x });
    }
  }

  function startSuikaChargeTick() {
    stopSuikaChargeTick();
    suikaChargeInterval = setInterval(() => {
      if (!document.getElementById('suika-container')?.offsetParent) return;
      updateSuikaRechargeBar();
    }, 100);
  }

  function stopSuikaChargeTick() {
    if (suikaChargeInterval) {
      clearInterval(suikaChargeInterval);
      suikaChargeInterval = null;
    }
  }

  window.showSuikaUI = showSuikaUI;
  window.renderSuika = renderSuika;
  window.suikaOnDropResult = function(success) {
    if (success && window.SoundManager) {
      window.SoundManager.playPianoNote(261.63, 0.08);
    }
  };

  (function() {
    const obs = new MutationObserver(() => {
      const container = document.getElementById('suika-container');
      if (container?.offsetParent) {
        startSuikaChargeTick();
      } else {
        stopSuikaChargeTick();
      }
    });
    obs.observe(document.body, { childList: true, subtree: true });
  })();
