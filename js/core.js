  // ── Supabase 클라이언트 초기화 ──────────────────────────────────────────────
  // anon key는 프론트엔드용 퍼블릭 키이므로 소스 노출이 허용됩니다.
  const _SB_URL = 'https://mfezndumovplmobazthw.supabase.co';
  const _SB_KEY = 'sb_publishable_hgb5-k3rlP3FYaDgAiF62g_UEG4oODU';
  const supabaseClient = supabase.createClient(_SB_URL, _SB_KEY);

  // ── State ──────────────────────────────────────────────────────────────────
  let actionCooldown = false;  // 게임 액션 광클 방지 (0.4초 쿨다운)
  let ws            = null;
  let currentRoomId = '';
  let currentUserId = '';  // 인증 후 표시명 (닉네임 또는 이메일)
  let currentUserEmail = '';  // 인증된 이메일 주소 (프로필 모달 표시용)
  let currentToken  = '';  // Supabase JWT access_token (auth 이후 설정)
  let pendingJoin   = null; // { roomId } — auth+connect 후 자동 입장
  let pendingNickname = null; // 회원가입 직후 설정할 닉네임
  let _signupNicknameChecked = false;   // 회원가입 시 중복확인 통과 여부
  let _profileNicknameChecked = false;  // 마이페이지 시 중복확인 통과 여부
  let _onNicknameCheckDone = null;
  let reconnectTimer = null;
  let isIntentionalLeave = false;
  let currentMode = 'lobby'; // 'lobby' | 'room'

  // ── 상수 (호이스팅/초기화 순서 문제 방지: 최상단 선언) ───────────────────────
  const games = ['omok', 'blackjack', 'tictactoe', 'connect4', 'indian', 'holdem', 'sevenpoker', 'thief', 'onecard', 'mahjong', 'alkkagi'];
  const RULES = {
    omok: {
        title: '🀱 오목 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>15×15 보드에서 <strong>가로·세로·대각선으로 5목</strong>을 먼저 완성하면 승리합니다.</li>
            <li>2명이 입장하면 무작위로 흑(선공)/백(후공)이 결정됩니다.</li>
        </ul>
        <h3>렌주룰 (흑 금수)</h3>
        <ul>
            <li><strong>쌍삼(3-3)</strong>: 활삼이 두 방향 이상 동시에 완성되는 곳 금지</li>
            <li><strong>쌍사(4-4)</strong>: 사(4목 위협)가 두 방향 이상 동시에 완성되는 곳 금지</li>
            <li><strong>장목(6목+)</strong>: 6개 이상 연속 금지</li>
            <li>백은 금수 없음, 6목 이상도 승리 인정</li>
        </ul>
        <h3>턴 타이머</h3>
        <ul>
            <li>한 턴당 <strong>15초</strong> 제한. 초과 시 시간 초과 패배.</li>
        </ul>
        <h3>리매치</h3>
        <ul>
            <li>게임 종료 후 <strong>🔄 한 판 더</strong> 버튼으로 리매치 가능. 흑/백을 교대하여 새 게임 시작.</li>
        </ul>
        <h3>전적 기록</h3>
        <ul>
            <li>게임 종료 시 승자에게 <strong>1승</strong>, 패자에게 <strong>1패</strong>가 전적에 기록됩니다.</li>
        </ul>`
    },
    blackjack: {
        title: '🃏 블랙잭 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>딜러 AI를 상대로 패의 합산이 <strong>21에 가장 가깝게</strong> 만드는 게임입니다.</li>
            <li><strong>❤️ 하트 10개 서바이벌:</strong> 플레이어와 딜러가 각각 10개 하트로 시작합니다.</li>
            <li>한쪽 하트가 0 이하가 되면 게임 종료. [한 판 더]로 리매치 가능합니다.</li>
        </ul>
        <h3>하트 증감</h3>
        <ul>
            <li>일반 승리: 플레이어 +1, 딜러 -1</li>
            <li>일반 패배: 플레이어 -1, 딜러 +1</li>
            <li>블랙잭 승리: 플레이어 +2, 딜러 -2 (보너스!)</li>
            <li>무승부(Push): 변동 없음</li>
        </ul>
        <h3>카드 점수</h3>
        <ul>
            <li>숫자 카드: 해당 숫자</li>
            <li>J, Q, K: 10점</li>
            <li>A: 1 또는 11 (버스트 방지 자동 조정)</li>
        </ul>
        <h3>딜러 규칙</h3>
        <ul>
            <li>딜러는 패의 합이 <strong>16 이하</strong>이면 반드시 카드를 추가로 뽑습니다.</li>
            <li>딜러 카드는 1장이 뒷면으로 숨겨져 있으며, Stand 후 공개됩니다.</li>
        </ul>`
    },
    tictactoe: {
        title: '⭕ 틱택토 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>3×3 보드에서 <strong>가로·세로·대각선으로 3목</strong>을 먼저 완성하면 승리합니다.</li>
            <li>2명이 입장하면 O(선공)와 X(후공)가 결정됩니다.</li>
        </ul>
        <h3>진행 방식</h3>
        <ul>
            <li>O와 X가 번갈아 가며 빈칸에 표시합니다.</li>
            <li>9칸이 모두 채워지고 승부가 나지 않으면 <strong>무승부</strong>입니다.</li>
        </ul>
        <h3>턴 타이머</h3>
        <ul>
            <li>한 턴당 <strong>15초</strong> 제한. 초과 시 시간 초과 패배.</li>
        </ul>
        <h3>리매치</h3>
        <ul>
            <li>게임 종료 후 <strong>🔄 한 판 더</strong> 버튼으로 리매치 가능. 선후공(O/X) 교대하여 새 게임 시작.</li>
        </ul>
        <h3>전적 기록</h3>
        <ul>
            <li>승리 시: <strong>1승</strong> 기록</li>
            <li>무승부: <strong>1무</strong> 기록</li>
            <li>패배 시: <strong>1패</strong> 기록</li>
        </ul>`
    },
    indian: {
        title: '🃏 인디언 포커 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>자신의 카드는 볼 수 없고, <strong>상대방의 카드만 볼 수 있습니다.</strong></li>
            <li>각 플레이어는 <strong>❤️×10 하트(인게임 체력)</strong>로 시작합니다. 하트는 라운드 진행을 위한 생명력이며, 0이 되면 매치에서 탈락합니다.</li>
        </ul>
        <h3>라운드 진행</h3>
        <ul>
            <li>매 라운드 각 플레이어는 카드를 1장 받습니다. 상대 카드를 보고 판단하세요.</li>
            <li>선공이 먼저 선택합니다:</li>
            <li>&nbsp;&nbsp;🏳️ <strong>포기</strong>: 선공 하트 -1. 라운드 종료.</li>
            <li>&nbsp;&nbsp;⚔️ <strong>승부</strong>: 후공에게 차례가 넘어갑니다.</li>
            <li>후공이 선택합니다:</li>
            <li>&nbsp;&nbsp;🏳️ <strong>포기</strong>: 후공 하트 -1. 라운드 종료.</li>
            <li>&nbsp;&nbsp;⚔️ <strong>승부(콜)</strong>: 카드를 공개하여 승패를 판정합니다.</li>
        </ul>
        <h3>라운드 내 하트 증감 (인게임 체력)</h3>
        <ul>
            <li>포기 시: 해당 플레이어 하트 -1.</li>
            <li>승부(콜) 후: 승자 하트 +2, 패자 하트 -2.</li>
            <li>숫자(2 < 3 < … < K < A)가 높은 쪽 승리. 동점 시 문양(♣ < ♦ < ♥ < ♠)으로 결정.</li>
        </ul>
        <h3>턴 타이머</h3>
        <ul>
            <li>한 턴당 <strong>30초</strong> 제한. 초과 시 <strong>포기</strong> 처리(하트 -1).</li>
        </ul>
        <h3>라운드마다 선후공 교체</h3>
        <ul>
            <li>매 라운드 선공과 후공이 교대합니다.</li>
        </ul>
        <h3>리매치</h3>
        <ul>
            <li>게임 완전 종료(누군가 하트 0개) 후 <strong>🔄 한 판 더</strong> 버튼으로 리매치 가능. 양쪽 하트를 10개로 리셋하여 새 게임 시작.</li>
        </ul>
        <h3>최종 전적 기록</h3>
        <ul>
            <li>누군가의 하트가 0이 되어 매치가 완전히 종료되면, 최종 생존자에게 <strong>1승</strong>, 탈락자에게 <strong>1패</strong>가 서버 전적에 기록됩니다.</li>
        </ul>`
    },
    sevenpoker: {
        title: '🃏 세븐 포커 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>최대 4인, 각 플레이어는 <strong>별(⭐)×10개</strong>로 시작합니다.</li>
            <li>커뮤니티 카드 없이 <strong>각자 7장</strong>을 받는 3~7구 분배 방식입니다.</li>
        </ul>
        <h3>카드 분배 (3~7구)</h3>
        <ul>
            <li><strong>3구</strong>: 3장 분배 (1~2번째 카드 히든, 3번째 공개)</li>
            <li><strong>4구</strong>: 4번째 카드 공개</li>
            <li><strong>5구</strong>: 5번째 카드 공개</li>
            <li><strong>6구</strong>: 6번째 카드 공개</li>
            <li><strong>7구</strong>: 7번째 카드 히든 (쇼다운 전까지 비공개)</li>
        </ul>
        <h3>액션</h3>
        <ul>
            <li>✅ <strong>체크</strong>: 팟에 별 1개 지불 (별 0개면 무료 체크)</li>
            <li>🏳️ <strong>폴드</strong>: 이번 라운드 포기</li>
        </ul>
        <h3>쇼다운 및 족보</h3>
        <ul>
            <li>7장 중 베스트 5장 족보로 승부.</li>
            <li>동점 시 팟 분할, 나머지는 다음 라운드 이월.</li>
        </ul>
        <h3>📋 포커 족보 순서</h3>
        <p>로티플 &gt; 스트레이트플러시 &gt; 포카드 &gt; 풀하우스 &gt; 플러시 &gt; 스트레이트 &gt; 트리플 &gt; 투페어 &gt; 원페어 &gt; 하이카드</p>
        <h3>매치 종료</h3>
        <ul>
            <li>파산자(별 0개) ≥ 전체 유저 수/2 이면 매치 종료.</li>
            <li>생존자 → 1승, 파산자 → 1패 전적 기록.</li>
        </ul>`
    },
    holdem: {
        title: '♠️ 텍사스 홀덤 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>최대 4인, 각 플레이어는 <strong>별(⭐)×10개</strong>로 시작합니다.</li>
            <li>전통적인 칩 베팅(레이즈/올인) 없이, 캐주얼한 <strong>별 서바이벌 룰</strong>을 적용합니다.</li>
        </ul>
        <h3>페이즈</h3>
        <ul>
            <li><strong>프리플랍</strong>: 개인 카드 2장 배분 → 체크/폴드</li>
            <li><strong>플랍</strong>: 커뮤니티 카드 3장 공개 → 체크/폴드</li>
            <li><strong>턴</strong>: 커뮤니티 카드 +1장 → 체크/폴드</li>
            <li><strong>리버</strong>: 커뮤니티 카드 +1장 → 체크/폴드</li>
            <li><strong>쇼다운</strong>: 생존자들의 7장(개인 2장+공유 5장)으로 족보 판정, 최고 족보가 팟 획득</li>
        </ul>
        <h3>액션</h3>
        <ul>
            <li>✅ <strong>체크</strong>: 팟에 별 1개 지불하고 다음 페이즈 진행. (별 0개면 무료 체크)</li>
            <li>🏳️ <strong>폴드</strong>: 이번 라운드 포기. (이미 낸 별은 돌려받지 못함)</li>
        </ul>
        <h3>쇼다운 및 족보</h3>
        <ul>
            <li>동점 시 팟을 n등분, 나머지는 다음 라운드로 이월</li>
        </ul>
        <h3>📋 포커 족보 순서</h3>
        <p>로티플 &gt; 스트레이트플러시 &gt; 포카드 &gt; 풀하우스 &gt; 플러시 &gt; 스트레이트 &gt; 트리플 &gt; 투페어 &gt; 원페어 &gt; 하이카드</p>
        <h3>매치 종료</h3>
        <ul>
            <li><strong>파산자 수 ≥ ceil(전체 유저 수 / 2)</strong>이면 매치 종료.</li>
            <li>생존자(별 보유) → <strong>1승</strong>, 파산자 → <strong>1패</strong> 전적 기록.</li>
        </ul>`
    },
    connect4: {
        title: '🔴🟡 4목 (Connect 4) 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>6행 × 7열 보드에서 <strong>가로·세로·대각선으로 4개</strong>를 먼저 이으면 승리합니다.</li>
            <li>2명이 입장하면 🔴 빨강(선공)과 🟡 노랑(후공)이 결정됩니다.</li>
        </ul>
        <h3>진행 방식</h3>
        <ul>
            <li>턴마다 열(1~7)을 하나 선택하면, 돌이 <strong>중력에 의해 해당 열의 가장 아래 빈 칸</strong>으로 떨어집니다.</li>
            <li>열이 꽉 찼으면 해당 열을 선택할 수 없습니다.</li>
            <li>42칸이 모두 차고 승부가 나지 않으면 <strong>무승부</strong>입니다.</li>
        </ul>
        <h3>턴 타이머</h3>
        <ul>
            <li>한 턴당 <strong>15초</strong> 제한. 초과 시 시간 초과 패배.</li>
        </ul>
        <h3>리매치</h3>
        <ul>
            <li>게임 종료 후 <strong>🔄 한 판 더</strong> 버튼으로 리매치 가능. 선후공(빨강/노랑) 교대하여 새 게임 시작.</li>
        </ul>
        <h3>전적 기록</h3>
        <ul>
            <li>승리 시: <strong>1승</strong> 기록</li>
            <li>무승부: <strong>1무</strong> 기록</li>
            <li>패배 시: <strong>1패</strong> 기록</li>
        </ul>`
    },
    thief: {
        title: '🃏 도둑잡기 (Thief) 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>2~4인, 52장+조커 1장 총 <strong>53장</strong>을 분배합니다.</li>
            <li>분배 직후 같은 숫자의 카드(페어)를 자동으로 제거합니다.</li>
        </ul>
        <h3>턴 진행</h3>
        <ul>
            <li>내 차례일 때 <strong>다음 생존 플레이어</strong>의 패에서 카드 1장을 무작위로 뽑아 옵니다.</li>
            <li>뽑은 직후 내 패에 같은 숫자가 생기면 즉시 페어로 버립니다.</li>
        </ul>
        <h3>승패</h3>
        <ul>
            <li>패가 0장이 되면 <strong>탈출(Win)</strong> — 턴에서 제외됩니다.</li>
            <li>최종적으로 <strong>조커 1장만 들고 남은 1명</strong>이 패배(Lose)하며 게임 종료.</li>
        </ul>`
    },
    onecard: {
        title: '🃏 원카드 (One Card) 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li>2~4인, 54장 덱(52장 + 흑백 조커 + 컬러 조커)에서 각 <strong>7장씩</strong> 분배.</li>
            <li>덱이 비면 버린 카드를 셔플하여 재활용합니다.</li>
        </ul>
        <h3>턴 진행</h3>
        <ul>
            <li>바닥 카드와 <strong>문양</strong> 또는 <strong>숫자</strong>가 일치하는 카드만 낼 수 있습니다.</li>
            <li>조커는 언제든 낼 수 있습니다.</li>
            <li>낼 카드가 없으면 덱에서 드로우합니다.</li>
        </ul>
        <h3>공격 카드</h3>
        <ul>
            <li><strong>A</strong>: +3장</li>
            <li><strong>흑백 조커(B)</strong>: +5장</li>
            <li><strong>컬러 조커(C)</strong>: +7장</li>
            <li>공격을 받으면 방어 카드로 막거나, 드로우를 선택해 패널티만큼 받습니다.</li>
            <li>방어: A→A/조커, 흑백 조커→컬러 조커만, 컬러 조커는 방어 불가.</li>
        </ul>
        <h3>특수 카드</h3>
        <ul>
            <li><strong>J</strong>: 다음 사람 턴 스킵</li>
            <li><strong>Q</strong>: 턴 진행 방향 반전</li>
            <li><strong>K</strong>: 한 번 더! (내가 다시 카드를 냄)</li>
            <li><strong>7</strong>: 다음 문양(♠♥♦♣) 강제 변경</li>
        </ul>
        <h3>원카드 콜</h3>
        <ul>
            <li>손패가 1장이 되면 <strong>원카드!</strong> 버튼이 활성화됩니다.</li>
            <li>본인이 먼저 누르면 안전해지고, 타인이 먼저 누르면 1장인 유저가 벌칙 1장 드로우.</li>
        </ul>
        <h3>파산 & 승패</h3>
        <ul>
            <li>손패가 <strong>20장 초과</strong> 시 즉시 파산(패배).</li>
            <li>패가 0장이 되면 <strong>승리</strong>, 나머지 <strong>패배</strong>.</li>
        </ul>`
    },
    mahjong: {
        title: '🀄 마작 (Mahjong) 룰 — Phase 1',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li><strong>4인 전용</strong>. 136장(수패 108장 + 자패 28장)을 사용합니다.</li>
            <li>전원 Ready 후 게임이 시작됩니다.</li>
        </ul>
        <h3>패 분배</h3>
        <ul>
            <li>4명에게 각각 <strong>13장</strong>씩 분배합니다.</li>
            <li>선(친)부터 턴이 시작되며, 턴 시작 시 <strong>쯔모</strong> 1장을 뽑아 14장이 됩니다.</li>
        </ul>
        <h3>타패 (Discard)</h3>
        <ul>
            <li>내 차례에 14장의 손패 중 1장을 선택해 버립니다.</li>
            <li>버린 후 다음 플레이어로 턴이 넘어가고, 그 플레이어가 자동으로 1장 쯔모합니다.</li>
        </ul>
        <h3>Phase 1 범위</h3>
        <ul>
            <li>치/퐁/깡 및 역 계산은 아직 구현되지 않았습니다.</li>
        </ul>`
    },
    alkkagi: {
        title: '⚫ 알까기 (Alkkagi) 룰',
        html: `
        <h3>게임 개요</h3>
        <ul>
            <li><strong>2인 대전</strong>. 흑돌 4개 vs 백돌 4개.</li>
            <li>돌을 튕겨 상대 알을 밀어내세요!</li>
        </ul>
        <h3>조작</h3>
        <ul>
            <li>내 차례에 돌을 드래그하여 당긴 뒤 놓으면 쏘아집니다. (향후 구현)</li>
            <li>클라이언트 주도권: 각자의 브라우저가 물리 연산을 수행합니다.</li>
        </ul>
        <h3>승리 조건</h3>
        <ul>
            <li>상대 알을 모두 보드 밖으로 밀어내면 승리!</li>
        </ul>`
    }
  };
  const POKER_HAND_RANKINGS_HTML = `
    <h3>📋 포커 족보 순서 (강 → 약)</h3>
    <ol style="margin:12px 0; padding-left:20px; line-height:2;">
      <li>로티플 (Royal Flush)</li>
      <li>스트레이트 플러시 (Straight Flush)</li>
      <li>포카드 (Four of a Kind)</li>
      <li>풀하우스 (Full House)</li>
      <li>플러시 (Flush)</li>
      <li>스트레이트 (Straight)</li>
      <li>트리플 (Three of a Kind)</li>
      <li>투페어 (Two Pair)</li>
      <li>원페어 (One Pair)</li>
      <li>하이카드 (High Card)</li>
    </ol>
  `;
  const GAME_VIEW_IDS = ['board-placeholder', 'gomoku-container', 'blackjack-container', 'tictactoe-container', 'connect4-container', 'indian-container', 'holdem-container', 'sevenpoker-container', 'thief-container', 'onecard-container', 'mahjong-container', 'alkkagi-container'];
  const PREFIX_TO_CONTAINER = { omok: 'gomoku-container', blackjack: 'blackjack-container', tictactoe: 'tictactoe-container', connect4: 'connect4-container', indian: 'indian-container', holdem: 'holdem-container', sevenpoker: 'sevenpoker-container', thief: 'thief-container', onecard: 'onecard-container', mahjong: 'mahjong-container', mahjong3: 'mahjong-container', alkkagi: 'alkkagi-container' };
  const GAME_STATE_HANDLERS = {
    tictactoe_state:  { logKey: 'ttt-state',       show: () => { if (typeof window.showTicTacToeUI === 'function') window.showTicTacToeUI(); },  render: (data) => { if (typeof window.renderTicTacToe === 'function') window.renderTicTacToe(data); } },
    connect4_state:   { logKey: 'c4-state',       show: () => { if (typeof window.showConnect4UI === 'function') window.showConnect4UI(); },   render: (data) => { if (typeof window.renderConnect4 === 'function') window.renderConnect4(data); } },
    indian_state:     { logKey: 'indian-state',   show: () => { if (typeof window.showIndianUI === 'function') window.showIndianUI(); },     render: (data) => { if (typeof window.renderIndian === 'function') window.renderIndian(data); } },
    holdem_state:     { logKey: 'holdem-state',   show: () => { if (typeof window.showHoldemUI === 'function') window.showHoldemUI(); },     render: (data) => { if (typeof window.renderHoldem === 'function') window.renderHoldem(data); } },
    sevenpoker_state: { logKey: 'sevenpoker-state', show: () => { if (typeof window.showSevenPokerUI === 'function') window.showSevenPokerUI(); }, render: (data) => { if (typeof window.renderSevenPoker === 'function') window.renderSevenPoker(data); } },
    thief_state:      { logKey: 'thief-state',    show: () => { if (typeof window.showThiefUI === 'function') window.showThiefUI(); },      render: (data) => { if (typeof window.renderThief === 'function') window.renderThief(data); } },
    onecard_state:    { logKey: 'onecard-state',  show: () => { if (typeof window.showOneCardUI === 'function') window.showOneCardUI(); },     render: (data) => { if (typeof window.renderOneCard === 'function') window.renderOneCard(data); } },
    mahjong_state:    { logKey: 'mahjong-state',  show: () => { if (typeof window.showMahjongUI === 'function') window.showMahjongUI(); },   render: (data) => { if (typeof window.renderMahjong === 'function') window.renderMahjong(data); } },
    mahjong3_state:   { logKey: 'mahjong3-state', show: () => { if (typeof window.showMahjongUI === 'function') window.showMahjongUI(); },   render: (data) => { if (typeof window.renderMahjong === 'function') window.renderMahjong(data); } },
    alkkagi_state:    { logKey: 'alkkagi-state',  show: () => { if (typeof window.showAlkkagiUI === 'function') window.showAlkkagiUI(); },    render: (data) => { if (typeof window.renderAlkkagi === 'function') window.renderAlkkagi(data); } },
  };
  const PREFIX_TO_TIMER = { omok: ['status-seconds', 'status-timer-block'], tictactoe: ['ttt-seconds', 'ttt-timer-block'], connect4: ['c4-seconds', 'c4-timer-block'], indian: ['indian-seconds', 'indian-timer-block'], holdem: ['holdem-seconds', 'holdem-timer-block'], sevenpoker: ['sevenpoker-seconds', 'sevenpoker-timer-block'], thief: ['thief-seconds', 'thief-timer-block'], onecard: ['onecard-seconds', 'onecard-timer-block'], mahjong: ['mahjong-seconds', 'mahjong-timer-block'], mahjong3: ['mahjong-seconds', 'mahjong-timer-block'] };

  // Debug panel element references
  const logOutput  = document.getElementById('log-output');
  const wsUrl      = document.getElementById('ws-url');
  const msgInput   = document.getElementById('msg-input');
  const inputUserId= document.getElementById('input-user-id');
  const inputRoomId= document.getElementById('input-room-id');

  window.onbeforeunload = function(e) {
    if (currentMode === 'room' && !isIntentionalLeave) {
      e.preventDefault();
      e.returnValue = '';
      return '';
    }
  };

  // ── 백그라운드 탭 복귀 시 연결 끊김 처리 ─────────────────────────────────────
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      // 로비 화면(room이 아닐 때)에서만 자동 재연결 허용
      if (currentMode !== 'room' && (!ws || ws.readyState !== WebSocket.OPEN)) {
        if (currentToken) connect();
        else window.location.reload();
      }
    }
  });

  // ── Init ──────────────────────────────────────────────────────────────────
  (function init() {
    setConnectionState(false);
    // 이미 로그인된 세션이 있으면 자동 복구
    supabaseClient.auth.getSession().then(({ data: { session } }) => {
      if (session) {
        currentToken     = session.access_token;
        currentUserEmail = session.user.email;
        currentUserId    = session.user.email;  // auth_ok에서 닉네임으로 갱신됨
        showLoggedIn(session.user.email);
        connect();
      }
    });
  })();

  // ── Supabase Auth 함수들 ──────────────────────────────────────────────────
  // ── Auth 모드 전환 ─────────────────────────────────────────────────────────
  let _authMode = 'login'; // 'login' | 'signup'

  function toggleAuthMode(mode) {
    _authMode = mode;
    const isSignup = mode === 'signup';
    document.getElementById('auth-panel-title').textContent  = isSignup ? '📝 회원가입' : '🔐 로그인';
    document.getElementById('auth-confirm-group').style.display = isSignup ? 'block' : 'none';
    document.getElementById('auth-nickname-group').style.display = isSignup ? 'block' : 'none';
    document.getElementById('auth-nickname').value = '';
    document.getElementById('auth-nickname-status').textContent = '';
    document.getElementById('auth-login-btn-row').style.display   = isSignup ? 'none' : '';
    document.getElementById('auth-login-switch').style.display    = isSignup ? 'none' : '';
    document.getElementById('auth-signup-btn-row').style.display  = isSignup ? '' : 'none';
    document.getElementById('auth-signup-switch').style.display   = isSignup ? '' : 'none';
    document.getElementById('auth-password').placeholder          = isSignup ? '비밀번호 (6자 이상)' : '비밀번호';
    document.getElementById('auth-password-confirm').value        = '';
    _signupNicknameChecked = false;
    document.getElementById('btn-auth-signup').disabled = true;
    setAuthStatus('');
  }

  /** 닉네임 입력 시 중복확인 강제 리셋 — 회원가입/마이페이지 공통 */
  function resetNicknameCheck(type) {
    if (type === 'signup') {
      _signupNicknameChecked = false;
      document.getElementById('btn-auth-signup').disabled = true;
      const statusEl = document.getElementById('auth-nickname-status');
      statusEl.textContent = '⚠️ 중복확인을 해주세요';
      statusEl.style.color = 'var(--warning)';
    } else if (type === 'profile') {
      _profileNicknameChecked = false;
      document.getElementById('btn-profile-apply').disabled = true;
      const statusEl = document.getElementById('profile-nickname-status');
      statusEl.textContent = '⚠️ 중복확인을 해주세요';
      statusEl.style.color = 'var(--warning)';
    }
  }

  function onAuthKeyDown(e) {
    if (e.key !== 'Enter') return;
    if (_authMode === 'signup') authSignup();
    else authLogin();
  }

  /** 회원가입 시 닉네임 중복확인 — Supabase 직접 조회 (WS 미연결 상태) */
  async function checkNickname() {
    const nickname = document.getElementById('auth-nickname').value.trim();
    const statusEl = document.getElementById('auth-nickname-status');
    if (!nickname) { statusEl.textContent = '닉네임을 입력하세요.'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname.length < 2 || nickname.length > 20) { statusEl.textContent = '닉네임은 2~20자로 입력하세요.'; statusEl.style.color = 'var(--danger)'; return; }
    statusEl.textContent = '확인 중...'; statusEl.style.color = 'var(--text-secondary)';
    try {
      const { data, error } = await supabaseClient.from('profiles').select('id').eq('username', nickname).limit(1);
      if (error) throw error;
      const available = !data || data.length === 0;
      _signupNicknameChecked = available;
      document.getElementById('btn-auth-signup').disabled = !available;
      if (available) { statusEl.textContent = '✓ 사용 가능한 닉네임입니다.'; statusEl.style.color = 'var(--success)'; }
      else { statusEl.textContent = '✗ 이미 사용 중인 닉네임입니다.'; statusEl.style.color = 'var(--danger)'; }
    } catch (e) {
      statusEl.textContent = '확인 중 오류가 발생했습니다.'; statusEl.style.color = 'var(--danger)';
    }
  }

  function sendSetNickname(nickname) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendRaw(JSON.stringify({ action: 'set_nickname', payload: { nickname } }));
  }

  function openProfileModal() {
    if (currentRoomId) {
      alert('게임 진행 중에는 프로필을 변경할 수 없습니다.');
      return;
    }
    document.getElementById('profile-email').textContent = currentUserEmail || currentUserId || '—';
    document.getElementById('profile-current-nickname').textContent = currentUserId || '—';
    document.getElementById('profile-new-nickname').value = currentUserId || '';
    _profileNicknameChecked = false;
    document.getElementById('btn-profile-apply').disabled = true;
    document.getElementById('profile-nickname-status').textContent = '⚠️ 중복확인을 해주세요';
    document.getElementById('profile-nickname-status').style.color = 'var(--warning)';
    document.getElementById('profile-modal').classList.add('show');
  }
  function closeProfileModal() {
    document.getElementById('profile-modal').classList.remove('show');
  }
  /** 마이페이지 닉네임 중복확인 — Supabase 직접 조회 */
  async function checkNicknameProfile() {
    const nickname = document.getElementById('profile-new-nickname').value.trim();
    const statusEl = document.getElementById('profile-nickname-status');
    if (!nickname) { statusEl.textContent = '닉네임을 입력하세요.'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname.length < 2 || nickname.length > 20) { statusEl.textContent = '닉네임은 2~20자로 입력하세요.'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname === currentUserId) {
      _profileNicknameChecked = true;
      document.getElementById('btn-profile-apply').disabled = false;
      statusEl.textContent = '✓ 현재 닉네임과 동일합니다.'; statusEl.style.color = 'var(--success)';
      return;
    }
    statusEl.textContent = '확인 중...'; statusEl.style.color = 'var(--text-secondary)';
    try {
      const { data, error } = await supabaseClient.from('profiles').select('id').eq('username', nickname).limit(1);
      if (error) throw error;
      const available = !data || data.length === 0;
      _profileNicknameChecked = available;
      document.getElementById('btn-profile-apply').disabled = !available;
      if (available) { statusEl.textContent = '✓ 사용 가능한 닉네임입니다.'; statusEl.style.color = 'var(--success)'; }
      else { statusEl.textContent = '✗ 이미 사용 중인 닉네임입니다.'; statusEl.style.color = 'var(--danger)'; }
    } catch (e) {
      statusEl.textContent = '확인 중 오류가 발생했습니다.'; statusEl.style.color = 'var(--danger)';
    }
  }
  function saveProfileNickname() {
    if (!_profileNicknameChecked) { showToast('중복확인을 먼저 해주세요.', 'error'); return; }
    const nickname = document.getElementById('profile-new-nickname').value.trim();
    if (!nickname || nickname.length < 2 || nickname.length > 20) {
      showToast('닉네임은 2~20자로 입력하세요.', 'error'); return;
    }
    if (!ws || ws.readyState !== WebSocket.OPEN) { showToast('연결이 끊어졌습니다.', 'error'); return; }
    sendRaw(JSON.stringify({ action: 'set_nickname', payload: { nickname } }));
    currentUserId = nickname;
    closeProfileModal();
    showToast('닉네임이 변경되었습니다.', 'info');
  }

  function requestOpponentRecord(userId) {
    if (!userId || userId === currentUserId) return;
    if (!ws || ws.readyState !== WebSocket.OPEN) { showToast('연결이 끊어졌습니다.', 'error'); return; }
    sendRaw(JSON.stringify({ action: 'get_user_record', payload: { userId } }));
  }
  function showOpponentRecordModal(userId, records) {
    const fmt = r => r ? `${r.wins}승 ${r.losses}패 ${r.draws}무` : '0승 0패 0무';
    const games = ['omok', 'tictactoe', 'connect4', 'holdem', 'sevenpoker', 'indian', 'blackjack_pve', 'blackjack', 'onecard', 'thief', 'mahjong', 'alkkagi'];
    const labels = { total: '전체', omok: '🀱 오목', tictactoe: '⭕❌ 틱택토', connect4: '🔴🟡 4목', holdem: '♠️ 텍사스 홀덤', sevenpoker: '🃏 세븐 포커', indian: '🃏 인디언 포커', blackjack_pve: '🃏 블랙잭 (PVE)', blackjack: '🃏 블랙잭 (PVP 레이드)', onecard: '🃏 원카드', thief: '🃏 도둑잡기', mahjong: '🀄 마작', alkkagi: '⚫ 알까기' };
    let html = '';
    html += `<div class="opponent-record-row"><span class="opponent-record-label">${labels.total}</span><span class="opponent-record-val">${fmt(records && records.total)}</span></div>`;
    for (const key of games) {
      const label = labels[key];
      if (label) {
        const r = records && records[key];
        html += `<div class="opponent-record-row"><span class="opponent-record-label">${label}</span><span class="opponent-record-val">${fmt(r)}</span></div>`;
      }
    }
    document.getElementById('opponent-record-title').textContent = `${userId} 전적`;
    document.getElementById('opponent-record-content').innerHTML = html || '<div>전적 없음</div>';
    document.getElementById('opponent-record-modal').classList.add('show');
  }
  function closeOpponentRecordModal() {
    document.getElementById('opponent-record-modal').classList.remove('show');
  }

  function setAuthStatus(msg, cls = '') {
    const el = document.getElementById('auth-status');
    el.textContent = msg;
    el.className   = cls;
  }

  async function authLogin() {
    const email    = document.getElementById('auth-email').value.trim();
    const password = document.getElementById('auth-password').value;
    if (!email || !password) { setAuthStatus('이메일과 비밀번호를 입력하세요.', 'error'); return; }
    setAuthStatus('로그인 중...', 'loading');
    const { data, error } = await supabaseClient.auth.signInWithPassword({ email, password });
    if (error) { setAuthStatus(error.message, 'error'); return; }
    currentToken     = data.session.access_token;
    currentUserEmail = data.user.email;
    currentUserId    = data.user.email;  // auth_ok에서 닉네임으로 갱신됨
    showLoggedIn(data.user.email);
    connect();
  }

  async function authSignup() {
    const email    = document.getElementById('auth-email').value.trim();
    const password = document.getElementById('auth-password').value;
    const confirm  = document.getElementById('auth-password-confirm').value;
    const nickname = document.getElementById('auth-nickname').value.trim();
    if (!email || !password) { setAuthStatus('이메일과 비밀번호를 입력하세요.', 'error'); return; }
    if (password.length < 6)  { setAuthStatus('비밀번호는 6자 이상이어야 합니다.', 'error'); return; }
    if (password !== confirm)  { setAuthStatus('비밀번호가 일치하지 않습니다.', 'error'); return; }
    if (!nickname || nickname.length < 2 || nickname.length > 20) {
      setAuthStatus('닉네임은 2~20자로 입력하세요.', 'error'); return;
    }
    if (!_signupNicknameChecked) {
      setAuthStatus('닉네임 중복확인을 해주세요.', 'error'); return;
    }
    setAuthStatus('가입 중...', 'loading');
    const { data, error } = await supabaseClient.auth.signUp({ email, password });
    if (error) { setAuthStatus(error.message, 'error'); return; }
    if (data.session) {
      currentToken     = data.session.access_token;
      currentUserEmail = data.user.email;
      currentUserId    = nickname;
      showLoggedIn(data.user.email, nickname);
      connect();
      pendingNickname = nickname;
    } else {
      setAuthStatus('📧 가입 확인 이메일을 발송했습니다. 이메일을 확인해 주세요.', 'loading');
    }
  }

  async function authLogout() {
    await supabaseClient.auth.signOut();
    currentToken  = '';
    currentUserId = '';
    if (ws) ws.close();
    showLoggedOut();
    setRoomState('', '');
    showToast('로그아웃 되었습니다.', 'info');
  }

  function showLoggedIn(email, nickname) {
    document.getElementById('auth-panel').style.display      = 'none';
    if (email) currentUserEmail = email;
    if (nickname) currentUserId = nickname;
    document.getElementById('game-cards').style.display      = 'flex';
    document.getElementById('btn-profile').style.display     = '';
    setAuthStatus('');
  }

  function showLoggedOut() {
    document.getElementById('auth-panel').style.display      = '';
    document.getElementById('game-cards').style.display      = 'none';
    document.getElementById('btn-profile').style.display     = 'none';
    document.getElementById('auth-email').value    = '';
    document.getElementById('auth-password').value = '';
  }

  // ── Connection State ───────────────────────────────────────────────────────
  function setConnectionState(connected) {
    const dot  = document.getElementById('conn-dot');
    const text = document.getElementById('conn-text');
    dot.className  = connected ? 'connected' : '';
    text.textContent = connected ? '연결됨' : '연결 끊김';

    document.getElementById('btn-connect').disabled    = connected;
    document.getElementById('btn-disconnect').disabled = !connected;
    document.getElementById('btn-send').disabled       = !connected;
    document.getElementById('btn-join').disabled       = !connected;

    if (connected) {
      document.getElementById('record-badge-wrapper').classList.add('visible');
    } else {
      document.getElementById('record-badge-wrapper').classList.remove('visible');
      document.getElementById('record-popup').classList.remove('open');
      setRoomState('', '');
    }
  }

  // ── Room State (lobby ↔ room transition) ───────────────────────────────────
  function setRoomState(userId, roomId) {
    if (userId) currentUserId = userId;
    currentRoomId = roomId;
    currentMode = roomId ? 'room' : 'lobby';

    if (roomId) {
      document.getElementById('lobby-view').style.display = 'none';
      document.getElementById('room-view').classList.add('active');
      document.getElementById('btn-leave').style.display = '';
      document.getElementById('chat-room-badge').textContent = roomId;
      document.getElementById('chat-input').disabled      = false;
      document.getElementById('btn-send-chat').disabled   = false;

      const titleEl = document.getElementById('game-area-title');
      if      (roomId.startsWith('omok'))      titleEl.textContent = '🀱 오목';
      else if (roomId.startsWith('blackjack')) titleEl.textContent = '🃏 블랙잭';
      else if (roomId.startsWith('tictactoe')) titleEl.textContent = '⭕❌ 틱택토';
      else if (roomId.startsWith('connect4'))  titleEl.textContent = '🔴🟡 4목';
      else if (roomId.startsWith('indian'))    titleEl.textContent = '🃏 인디언 포커';
      else if (roomId.startsWith('holdem'))   titleEl.textContent = '♠️ 텍사스 홀덤';
      else if (roomId.startsWith('sevenpoker')) titleEl.textContent = '🃏 세븐 포커';
      else if (roomId.startsWith('thief'))     titleEl.textContent = '🃏 도둑잡기';
      else if (roomId.startsWith('onecard'))   titleEl.textContent = '🃏 원카드';
      else if (roomId.startsWith('mahjong3'))  titleEl.textContent = '🀄 마작 (3인)';
      else if (roomId.startsWith('mahjong'))   titleEl.textContent = '🀄 마작 (4인)';
      else if (roomId.startsWith('alkkagi'))   titleEl.textContent = '⚫ 알까기';
      else                                     titleEl.textContent = roomId;

      const codeParts = roomId.split('_');
      const roomCode  = codeParts[codeParts.length - 1].toUpperCase();
      document.getElementById('room-code-text').textContent = roomCode;
      document.getElementById('room-code-badge').classList.add('visible');

      const rulesBtn = document.getElementById('btn-ingame-rules');
      rulesBtn.style.display = (roomId.startsWith('omok') || roomId.startsWith('blackjack') || roomId.startsWith('tictactoe') || roomId.startsWith('connect4') || roomId.startsWith('indian') || roomId.startsWith('holdem') || roomId.startsWith('sevenpoker') || roomId.startsWith('thief') || roomId.startsWith('onecard') || roomId.startsWith('mahjong') || roomId.startsWith('mahjong3') || roomId.startsWith('alkkagi')) ? '' : 'none';
      const addBotBtn = document.getElementById('btn-add-bot');
      addBotBtn.style.display = (roomId.startsWith('omok') || roomId.startsWith('connect4') || roomId.startsWith('tictactoe') || roomId.startsWith('indian') || roomId.startsWith('holdem') || roomId.startsWith('sevenpoker') || roomId.startsWith('thief') || roomId.startsWith('onecard') || roomId.startsWith('mahjong') || roomId.startsWith('mahjong3') || roomId.startsWith('alkkagi')) ? '' : 'none';

      if (inputUserId) inputUserId.value = userId;
      if (inputRoomId) inputRoomId.value = roomId;

      const readyArea = document.getElementById('ready-area');
      const btnReady = document.getElementById('btn-ready');
      const readyCountEl = document.getElementById('ready-count');
      const readyHintEl = document.getElementById('ready-hint');
      if (!roomId.startsWith('blackjack')) {
        readyArea.style.display = 'flex';
        if (btnReady) btnReady.disabled = false;
        if (readyCountEl) readyCountEl.textContent = roomId.startsWith('mahjong3') ? '0/3' : (roomId.startsWith('mahjong') ? '0/4' : '0/0');
        if (readyHintEl) readyHintEl.textContent = roomId.startsWith('mahjong3') ? '3인이 모두 준비해야 게임이 시작됩니다' : (roomId.startsWith('mahjong') ? '4인이 모두 준비해야 게임이 시작됩니다' : '전원이 준비해야 게임이 시작됩니다');
      } else {
        readyArea.style.display = 'none';
      }

    } else {
      document.getElementById('lobby-view').style.display = '';
      document.getElementById('room-view').classList.remove('active');
      document.getElementById('btn-leave').style.display  = 'none';
      document.getElementById('room-code-badge').classList.remove('visible');
      document.getElementById('room-code-text').textContent = '------';
      document.getElementById('btn-ingame-rules').style.display = 'none';
      document.getElementById('btn-add-bot').style.display = 'none';
      document.getElementById('btn-takeover').style.display = 'none';
      document.getElementById('chat-input').disabled      = true;
      document.getElementById('btn-send-chat').disabled   = true;

      gomokuTurnUserId = '';
      gomokuMyColor    = 0;
      gomokuColorMap   = {};
      gomokuBoardReady = false;
      gomokuEnded      = false;
      gomokuPrevBoard  = null;
      tttBoardReady    = false;
      tttPrevBoard     = null;
      c4BoardReady     = false;
      c4PrevBoard      = null;
      if (typeof window.setGomokuEnded === 'function') window.setGomokuEnded();
      document.getElementById('gomoku-container').style.display    = 'none';
      document.getElementById('blackjack-container').style.display = 'none';
      document.getElementById('tictactoe-container').style.display = 'none';
      document.getElementById('connect4-container').style.display  = 'none';
      document.getElementById('indian-container').style.display    = 'none';
      document.getElementById('holdem-container').style.display    = 'none';
      document.getElementById('sevenpoker-container').style.display = 'none';
      document.getElementById('thief-container').style.display    = 'none';
      document.getElementById('onecard-container').style.display   = 'none';
      document.getElementById('mahjong-container').style.display   = 'none';
      document.getElementById('alkkagi-container').style.display   = 'none';
      document.getElementById('board-placeholder').style.display   = 'flex';
      const rematchArea = document.getElementById('rematch-area');
      if (rematchArea) {
        rematchArea.classList.remove('visible');
        rematchArea.style.display = 'none';
      }
      document.getElementById('rematch-count').textContent = '0/0';
      document.getElementById('btn-rematch').disabled = false;
      document.getElementById('ready-area').style.display = 'none';
      document.getElementById('ready-count').textContent = '0/0';
      document.getElementById('btn-ready').disabled = false;
      document.getElementById('chat-messages').innerHTML = '';
    }
  }

  /** record_update 수신 시 전적 배지 및 팝업을 갱신합니다. */
  function updateRecords(records) {
    if (!records) return;
    const fmt = r => r ? `${r.wins}승 ${r.losses}패 ${r.draws}무` : '0승 0패 0무';
    if (records.total)      document.getElementById('record-total').textContent      = fmt(records.total);
    if (records.omok)       document.getElementById('record-omok').textContent       = fmt(records.omok);
    if (records.blackjack_pve) document.getElementById('record-blackjack-pve').textContent = fmt(records.blackjack_pve);
    if (records.blackjack)   document.getElementById('record-blackjack').textContent   = fmt(records.blackjack);
    if (records.tictactoe)  document.getElementById('record-tictactoe').textContent = fmt(records.tictactoe);
    if (records.connect4)   document.getElementById('record-connect4').textContent   = fmt(records.connect4);
    if (records.indian)     document.getElementById('record-indian').textContent     = fmt(records.indian);
    if (records.holdem)     document.getElementById('record-holdem').textContent     = fmt(records.holdem);
    if (records.sevenpoker) document.getElementById('record-sevenpoker').textContent = fmt(records.sevenpoker);
    if (records.thief)      document.getElementById('record-thief').textContent      = fmt(records.thief);
    if (records.onecard)   document.getElementById('record-onecard').textContent    = fmt(records.onecard);
    if (records.mahjong)  document.getElementById('record-mahjong').textContent   = fmt(records.mahjong);
    if (records.alkkagi)  document.getElementById('record-alkkagi').textContent  = fmt(records.alkkagi);
  }

  /** 채팅창 접기/펼치기 토글 */
  function toggleChatCollapse() {
    const panel = document.getElementById('chat-panel');
    const btn   = document.getElementById('btn-chat-toggle');
    panel.classList.toggle('collapsed');
    btn.textContent = panel.classList.contains('collapsed') ? '채팅창 열기 💬' : '채팅 닫기 🔽';
    btn.title = panel.classList.contains('collapsed') ? '채팅창 펼치기' : '채팅창 접기';
  }

  /** 전적 팝업 토글 (외부 클릭 시 닫힘) */
  function toggleRecordPopup(e) {
    e.stopPropagation();
    document.getElementById('record-popup').classList.toggle('open');
  }
  document.addEventListener('click', () => {
    document.getElementById('record-popup')?.classList.remove('open');
  });

  // ── Chat System ────────────────────────────────────────────────────────────
  function addChatMessage(type, parsed) {
    const messages = document.getElementById('chat-messages');
    const el = document.createElement('div');

    if (type === 'chat') {
      const isMine  = parsed.userId === currentUserId;
      const content = (parsed.message || '').replace(/^\[.+?\]:\s*/, '');
      el.className  = `chat-msg ${isMine ? 'mine' : 'other'}`;
      el.innerHTML  = isMine
        ? `<div class="chat-bubble">${escapeHTML(content)}</div>`
        : `<div class="chat-sender">${escapeHTML(parsed.userId || '')}</div>`
        + `<div class="chat-bubble">${escapeHTML(content)}</div>`;
    } else if (type === 'join') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="system-msg">👋 ${escapeHTML(parsed.userId || '')}님이 입장했습니다</div>`;
    } else if (type === 'leave') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="system-msg">🚪 ${escapeHTML(parsed.userId || '')}님이 퇴장했습니다</div>`;
    } else if (type === 'game_result') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="result-msg">🏆 ${escapeHTML(parsed.message || '')}</div>`;
    } else if (type === 'game_notice') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="notice-msg">${escapeHTML(parsed.message || '')}</div>`;
    } else if (type === 'system') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="system-msg">✦ ${escapeHTML(parsed.message || '')}</div>`;
    }

    messages.appendChild(el);
    messages.scrollTop = messages.scrollHeight;

    const previewEl = document.getElementById('chat-preview');
    if (previewEl) {
      let text = '';
      if (type === 'chat') {
        text = (parsed.message || '').replace(/^\[.+?\]:\s*/, '');
      } else if (type === 'join') {
        text = `${parsed.userId || ''}님이 입장했습니다`;
      } else if (type === 'leave') {
        text = `${parsed.userId || ''}님이 퇴장했습니다`;
      } else if (type === 'game_result') {
        text = parsed.message || '';
      } else if (type === 'game_notice') {
        text = parsed.message || '';
      } else if (type === 'system') {
        text = parsed.message || '';
      }
      if (text) {
        previewEl.textContent = text.replace(/<[^>]+>/g, '').trim();
      }
    }
  }

  function sendChat() {
    const input   = document.getElementById('chat-input');
    const message = input.value.trim();
    if (!message || !ws || ws.readyState !== WebSocket.OPEN || !currentRoomId) return;
    sendRaw(JSON.stringify({ action: 'chat', payload: { message } }));
    input.value = '';
  }

  function onChatKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChat(); }
  }

  // ── Toast / Modal ──────────────────────────────────────────────────────────
  function showToast(message, type = 'error') {
    const container = document.getElementById('toast-container');
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.textContent = message;
    el.onclick = () => dismissToast(el);
    container.appendChild(el);
    const timer = setTimeout(() => dismissToast(el), 4000);
    el._timer = timer;
  }
  function dismissToast(el) {
    if (!el.parentNode) return;
    clearTimeout(el._timer);
    el.classList.add('dismissing');
    el.addEventListener('animationend', () => el.remove(), { once: true });
  }
  function showResultModal(message) {
    document.getElementById('result-modal-msg').innerHTML = message.replace(/\n/g, '<br>');
    const modal = document.getElementById('result-modal');
    modal.classList.add('show');
    clearTimeout(modal._autoClose);
    modal._autoClose = setTimeout(closeResultModal, 7000);
  }
  function closeResultModal() {
    const modal = document.getElementById('result-modal');
    modal.classList.remove('show');
    clearTimeout(modal._autoClose);
  }

  /** 현재 입장한 방의 접두사에 맞는 룰 모달을 표시합니다. */
  function showCurrentRules() {
    const prefix = currentRoomId.startsWith('omok')      ? 'omok'
                 : currentRoomId.startsWith('blackjack') ? 'blackjack'
                 : currentRoomId.startsWith('tictactoe') ? 'tictactoe'
                 : currentRoomId.startsWith('connect4')  ? 'connect4'
                 : currentRoomId.startsWith('indian')   ? 'indian'
                 : currentRoomId.startsWith('holdem')   ? 'holdem'
                 : currentRoomId.startsWith('sevenpoker') ? 'sevenpoker'
                 : currentRoomId.startsWith('thief')    ? 'thief'
                 : currentRoomId.startsWith('onecard')  ? 'onecard'
                 : currentRoomId.startsWith('mahjong3')  ? 'mahjong'
                 : currentRoomId.startsWith('mahjong')   ? 'mahjong'
                 : currentRoomId.startsWith('alkkagi')   ? 'alkkagi'
                 : null;
    if (prefix) showRules(prefix);
  }

  // ── Rules Modal ────────────────────────────────────────────────────────────
  function showRules(game) {
    const rule = RULES[game];
    if (!rule) return;
    document.getElementById('rules-title').textContent  = rule.title;
    document.getElementById('rules-content').innerHTML  = rule.html;
    document.getElementById('rules-modal').classList.add('show');
  }

  function closeRules() {
    document.getElementById('rules-modal').classList.remove('show');
  }

  // ── Debug Panel ────────────────────────────────────────────────────────────
  function toggleDebugPanel() {
    document.getElementById('debug-panel').classList.toggle('open');
    document.getElementById('debug-backdrop').classList.toggle('open');
  }

  // ── Log (debug only) ───────────────────────────────────────────────────────
  function escapeHTML(str) {
    return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
  function escapeForJsAttr(str) {
    return String(str).replace(/\\/g,'\\\\').replace(/"/g,'\\"').replace(/'/g,"\\'");
  }
  function formatJSON(raw) {
    try { return JSON.stringify(JSON.parse(raw), null, 2); } catch { return raw; }
  }
  function addLog(type, data) {
    const now  = new Date();
    const time = now.toLocaleTimeString('ko-KR', { hour12: false })
               + '.' + String(now.getMilliseconds()).padStart(3,'0');
    const LABELS = { sent:'↑ SENT', recv:'↓ RECV', error:'✖ ERROR', info:'ℹ INFO',
                     system:'✦ SYS', chat:'💬 CHAT', 'join-log':'→ JOIN', 'leave-log':'← LEAVE',
                     'game-result':'🎲 GAME', 'game-notice':'📢 NOTICE', 'board-update':'🀱 BOARD',
                     record:'🏆 RECORD',
                     'bj-state':'🃏 BJ', 'bj-dealer':'🤖 DEALER' };
    const label = LABELS[type] || type.toUpperCase();
    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span class="log-time">${time}</span> <strong>${label}</strong> ${escapeHTML(formatJSON(data)).slice(0,300)}`;
    logOutput.appendChild(entry);
    logOutput.scrollTop = logOutput.scrollHeight;
  }
  function clearLog() { logOutput.innerHTML = ''; }

  // ── WebSocket ──────────────────────────────────────────────────────────────
  function connect() {
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    const url = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host + '/ws';
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
    const dot = document.getElementById('conn-dot');
    dot.className = 'connecting';
    document.getElementById('conn-text').textContent = '연결 중...';
    try {
      ws = new WebSocket(url);
      ws.onopen = () => {
        isIntentionalLeave = false;
        setConnectionState(true);
        addLog('info', `연결 성공 → ${url}`);
        if (currentToken) {
          sendRaw(JSON.stringify({ action: 'auth', payload: { token: currentToken } }));
        }
      };
      ws.onmessage = (event) => {
        const raw = event.data;
        try {
          const parsed = JSON.parse(raw);
          switch (parsed.type) {
            case 'auth_ok':
              addLog('info', raw);
              if (parsed.userId) currentUserId = parsed.userId;
              document.getElementById('btn-profile').style.display = '';
              if (pendingJoin) {
                sendJoin(pendingJoin.roomId);
                pendingJoin = null;
              } else if (pendingNickname) {
                sendSetNickname(pendingNickname);
                pendingNickname = null;
              }
              break;
            case 'nickname_check':
              if (typeof _onNicknameCheckDone === 'function') _onNicknameCheckDone(parsed.available);
              break;
            case 'opponent_record':
              showOpponentRecordModal(parsed.userId, parsed.records);
              break;
            case 'system':
              addLog('system', raw);
              addChatMessage('system', parsed);
              break;
            case 'error': {
              addLog('error', raw);
              let errMsg = parsed.message || '서버 오류가 발생했습니다.';
              if (currentRoomId && currentRoomId.startsWith('mahjong3') && /3인|인원/.test(errMsg)) {
                errMsg = '3인 마작입니다. 인원을 기다려주세요.';
                const btnReady = document.getElementById('btn-ready');
                if (btnReady) btnReady.disabled = false;
              } else if (currentRoomId && currentRoomId.startsWith('mahjong') && /4명|인원/.test(errMsg)) {
                errMsg = '4인 대전 전용 게임입니다. 인원을 기다려주세요.';
                const btnReady = document.getElementById('btn-ready');
                if (btnReady) btnReady.disabled = false;
              }
              showToast(errMsg, 'error');
              const fullKeywords = ['이미 2명', '1인 전용', '가득 찼습니다', '정원 초과', '인원이 가득', '방이 해산'];
              if (fullKeywords.some(kw => errMsg.includes(kw))) {
                leaveRoom(true);
              }
              break;
            }
            case 'chat':
              addLog('chat', raw);
              addChatMessage('chat', parsed);
              break;
            case 'join':
              addLog('join-log', raw);
              addChatMessage('join', parsed);
              if (parsed.userId === currentUserId) {
                setRoomState(parsed.userId, parsed.roomId);
              }
              break;
            case 'leave':
              addLog('leave-log', raw);
              addChatMessage('leave', parsed);
              if (parsed.userId === currentUserId) {
                setRoomState('', '');
              }
              break;
            case 'board_update':
              addLog('board-update', JSON.stringify({ type: parsed.type, turn: parsed.data?.turn }));
              document.getElementById('btn-takeover').style.display = 'none';
              ['status-turn-user', 'ttt-status', 'c4-status'].forEach(id => {
                const el = document.getElementById(id);
                if (el) { el.style.color = ''; el.style.fontWeight = ''; }
              });
              showGomokuBoard();
              renderBoard(parsed.data);
              break;
            case 'timer_tick':
              updateGameTimer(parsed.turnUser, parsed.remaining);
              break;
            case 'game_paused': {
              const playerIds = parsed.playerIds || [];
              const isPlayer = playerIds.includes(currentUserId);
              const statusEls = ['status-turn-user', 'ttt-status', 'c4-status', 'indian-status', 'holdem-status', 'sevenpoker-status', 'thief-status', 'onecard-status'];
              statusEls.forEach(id => {
                const el = document.getElementById(id);
                if (el) {
                  el.textContent = '🚨 플레이어 퇴장. 난입 대기 중...';
                  el.style.color = 'var(--danger, #dc2626)';
                  el.style.fontWeight = '700';
                }
              });
              const btnTakeover = document.getElementById('btn-takeover');
              if (btnTakeover) btnTakeover.style.display = isPlayer ? 'none' : '';
              break;
            }
            case 'game_result':
              addLog('game-result', raw);
              addChatMessage('game_result', parsed);
              showResultModal(parsed.message || '게임이 종료되었습니다');
              if (parsed.data && parsed.data.board) {
                showGomokuBoard();
                renderBoard({ board: parsed.data.board, turn: '', colors: parsed.data.colors || {}, lastMove: parsed.data.lastMove || [-1,-1] });
                if (typeof window.setGomokuEnded === 'function') window.setGomokuEnded();
              }
              if (parsed.rematchEnabled) {
                const total = parsed.data?.totalCount ?? 2;
                document.getElementById('rematch-count').textContent = `0/${total}`;
                document.getElementById('btn-rematch').disabled = false;
                const rematchArea = document.getElementById('rematch-area');
                rematchArea.style.display = 'flex';
                rematchArea.classList.add('visible');
              }
              break;
            case 'game_notice':
              addLog('game-notice', raw);
              addChatMessage('game_notice', parsed);
              break;
            case 'ready_update': {
              const ready = parsed.readyCount ?? 0;
              const total = parsed.totalCount ?? 0;
              document.getElementById('ready-count').textContent = `${ready}/${total}`;
              if (ready >= total && total > 1) {
                document.getElementById('ready-area').style.display = 'none';
                document.getElementById('btn-ready').disabled = false;
              }
              break;
            }
            case 'rematch_update': {
              const ready = parsed.readyCount ?? 0;
              const total = parsed.totalCount ?? 2;
              document.getElementById('rematch-count').textContent = `${ready}/${total}`;
              if (ready >= total && total > 1) {
                document.getElementById('rematch-area').style.display = 'none';
                document.getElementById('rematch-area').classList.remove('visible');
                document.getElementById('btn-rematch').disabled = false;
                if (currentRoomId.startsWith('omok')) {
                  gomokuEnded = false;
                  document.getElementById('gomoku-spectator-msg').style.display = 'none';
                }
              }
              break;
            }
            case 'blackjack_state':
              addLog('bj-state', raw);
              showBlackjackUI();
              renderBlackjackState(parsed.data);
              break;
            case 'blackjack_pvp_state':
              addLog('bj-pvp-state', raw);
              showBlackjackUI();
              if (typeof window.renderBlackjackPVPState === 'function') {
                window.renderBlackjackPVPState(parsed.data);
              } else if (typeof window.adaptPVPToPVEData === 'function') {
                renderBlackjackState(window.adaptPVPToPVEData(parsed.data));
              } else {
                renderBlackjackState(parsed.data);
              }
              break;
            case 'dealer_action':
              addLog('bj-dealer', raw);
              if (parsed.data && parsed.data.players) {
                if (typeof window.renderBlackjackPVPState === 'function') {
                  window.renderBlackjackPVPState(parsed.data);
                } else if (typeof window.adaptPVPToPVEData === 'function') {
                  renderBlackjackState(window.adaptPVPToPVEData(parsed.data));
                } else {
                  renderBlackjackState(parsed.data);
                }
              } else {
                renderBlackjackState(parsed.data);
              }
              break;
            case 'thief_hover':
              if (parsed.targetId != null && typeof parsed.index === 'number') {
                thiefHoveredTargetId = parsed.targetId;
                thiefHoveredIndex = parsed.index;
                if (lastThiefState) renderThief(lastThiefState);
              }
              break;
            case 'alkkagi_flick':
              addLog('alkkagi-flick', raw);
              if (typeof window.handleAlkkagiFlick === 'function') window.handleAlkkagiFlick(parsed.data);
              break;
            case 'tictactoe_state': case 'connect4_state': case 'indian_state': case 'holdem_state': case 'sevenpoker_state': case 'thief_state': case 'onecard_state': case 'mahjong_state': case 'mahjong3_state': case 'alkkagi_state': {
              document.getElementById('btn-takeover').style.display = 'none';
              ['status-turn-user', 'ttt-status', 'c4-status', 'indian-status', 'thief-status', 'onecard-status', 'mahjong-status', 'alkkagi-status'].forEach(id => {
                const el = document.getElementById(id);
                if (el) { el.style.color = ''; el.style.fontWeight = ''; }
              });
              if (parsed.type === 'thief_state') { thiefHoveredTargetId = ''; thiefHoveredIndex = -1; }
              const h = GAME_STATE_HANDLERS[parsed.type];
              if (h) { addLog(h.logKey, raw); h.show(); h.render(parsed.data); }
              if (parsed.data?.canTakeover) {
                document.getElementById('btn-takeover').style.display = '';
              }
              break;
            }
            case 'indian_showdown_result':
              addLog('indian-showdown', raw);
              showIndianShowdownOverlay(parsed.data);
              break;
            case 'poker_showdown_result':
              addLog('poker-showdown', raw);
              showPokerShowdownOverlay(parsed.data);
              break;
            case 'record_update': {
              addLog('record', raw);
              updateRecords(parsed.records);
              break;
            }
            default:
              addLog('recv', raw);
          }
        } catch {
          addLog('recv', raw);
        }
      };
      ws.onerror = () => {
        addLog('error', '연결 오류. 서버가 실행 중인지 확인하세요.');
        document.getElementById('conn-dot').className  = '';
        document.getElementById('conn-text').textContent = '연결 오류';
      };
      ws.onclose = (event) => {
        setConnectionState(false);
        addLog('info', `연결 종료 (code: ${event.code})`);
        ws = null;
        if (currentToken && currentMode !== 'room' && !isIntentionalLeave) {
          reconnectTimer = setTimeout(() => {
            addLog('info', '서버에 재연결을 시도합니다...');
            connect();
          }, 3000);
        }
      };
    } catch (e) {
      addLog('error', `WebSocket 생성 실패: ${e.message}`);
    }
  }

  function disconnect() {
    if (ws) ws.close(1000, 'User disconnected');
  }

  function genRoomCode() {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    return Array.from({ length: 6 }, () => chars[Math.floor(Math.random() * chars.length)]).join('');
  }

  function createRoom(prefix) {
    if (!currentUserId) { showToast('먼저 로그인하세요.', 'error'); return; }
    const code   = genRoomCode();
    const roomId = prefix + '_' + code;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      pendingJoin = { roomId };
      connect();
    } else {
      sendJoin(roomId);
    }
  }

  function startPVE(prefix) {
    if (!currentUserId) { showToast('먼저 로그인하세요.', 'error'); return; }
    const code   = genRoomCode();
    const roomId = prefix + '_pve_' + code;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      pendingJoin = { roomId };
      connect();
    } else {
      sendJoin(roomId);
    }
  }

  function joinWithCode(prefix, inputId) {
    if (!currentUserId) { showToast('먼저 로그인하세요.', 'error'); return; }
    const input = document.getElementById(inputId);
    const code  = (input.value || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
    input.value = code;
    if (code.length !== 6) {
      showToast('코드는 영문+숫자 6자리를 입력하세요.', 'error');
      input.focus();
      return;
    }
    const roomId = prefix + '_' + code;
    input.value  = '';
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      pendingJoin = { roomId };
      connect();
    } else {
      sendJoin(roomId);
    }
  }

  function sendJoin(roomId) {
    sendRaw(JSON.stringify({ action: 'join', payload: { roomId, userId: currentUserId } }));
  }

  function requestAddBot() {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      showToast('연결이 끊어졌습니다.', 'error');
      return;
    }
    sendRaw(JSON.stringify({ action: 'add_bot', payload: {} }));
    showToast('AI 봇 추가 요청을 보냈습니다.', 'info');
  }

  function copyRoomCode() {
    const code = document.getElementById('room-code-text').textContent.trim();
    if (!code || code === '------') return;
    navigator.clipboard.writeText(code).then(() => {
      showToast('방 코드가 복사되었습니다!', 'success');
    }).catch(() => {
      const ta = document.createElement('textarea');
      ta.value = code;
      ta.style.cssText = 'position:fixed;opacity:0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
      showToast('방 코드가 복사되었습니다!', 'success');
    });
  }

  function leaveRoom(skipConfirm) {
    isIntentionalLeave = true;
    if (!skipConfirm && !confirm('게임 진행 중에 나가면 패배로 기록될 수 있습니다.\n정말 나가시겠습니까?')) {
      isIntentionalLeave = false;
      return;
    }
    sendRaw(JSON.stringify({ action: 'leave', payload: {} }));
    setRoomState('', '');
  }

  function debugJoinRoom() {
    const userId = inputUserId.value.trim();
    const roomId = inputRoomId.value.trim();
    if (!userId || !roomId) return;
    currentUserId = userId;
    sendRaw(JSON.stringify({ action: 'join', payload: { roomId, userId } }));
    toggleDebugPanel();
  }

  function sendRaw(text) {
    if (!text || !ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(text);
    addLog('sent', text);
  }

  /** 게임 액션 전송 (광클 방지: 0.4초 쿨다운 적용) */
  function sendGameAction(payload) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (actionCooldown) return;
    actionCooldown = true;
    setTimeout(() => { actionCooldown = false; }, 400);
    sendRaw(JSON.stringify({ action: 'game_action', payload }));
  }

  function sendMessage() {
    const text = msgInput.value.trim();
    if (!text) return;
    try { JSON.parse(text); } catch { addLog('error', '잘못된 JSON'); return; }
    sendRaw(text);
  }

  msgInput.addEventListener('keydown', (e) => {
    if (e.ctrlKey && e.key === 'Enter') sendMessage();
  });

  function sendReady() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    let minPlayers = 2;
    if (currentRoomId.startsWith('mahjong3')) minPlayers = 3;
    else if (currentRoomId.startsWith('mahjong')) minPlayers = 4;
    const readyCountEl = document.getElementById('ready-count');
    const total = readyCountEl ? parseInt((readyCountEl.textContent || '0/0').split('/')[1], 10) || 0 : 0;
    if (total < minPlayers) {
      alert('최소 인원(' + minPlayers + '명)이 모여야 준비할 수 있습니다. 봇을 추가하거나 다른 유저를 기다려주세요.');
      return;
    }
    sendGameAction({ cmd: 'ready' });
    document.getElementById('btn-ready').disabled = true;
  }

  function sendRematch() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'rematch' });
    document.getElementById('btn-rematch').disabled = true;
  }

  function sendTakeover() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'takeover' });
  }

  // ── 게임 뷰 전환 (DRY) ─────────────────────────────────────────────────────
  function switchGameView(prefix) {
    const showId = PREFIX_TO_CONTAINER[prefix] || 'board-placeholder';
    GAME_VIEW_IDS.forEach(id => {
      const el = document.getElementById(id);
      if (el) el.style.display = id === showId ? 'flex' : 'none';
    });
    const rematchEl = document.getElementById('rematch-area');
    if (rematchEl) {
      rematchEl.classList.remove('visible');
      rematchEl.style.display = 'none';
    }
  }

  function updateGameTimer(turnUser, remaining) {
    const prefix = currentRoomId.startsWith('omok') ? 'omok'
                 : currentRoomId.startsWith('tictactoe') ? 'tictactoe'
                 : currentRoomId.startsWith('connect4') ? 'connect4'
                 : currentRoomId.startsWith('indian') ? 'indian'
                 : currentRoomId.startsWith('holdem') ? 'holdem'
                 : currentRoomId.startsWith('sevenpoker') ? 'sevenpoker'
                 : currentRoomId.startsWith('thief') ? 'thief'
                 : currentRoomId.startsWith('onecard') ? 'onecard'
                 : currentRoomId.startsWith('mahjong3') ? 'mahjong3'
                 : currentRoomId.startsWith('mahjong') ? 'mahjong'
                 : null;
    if (!prefix || !PREFIX_TO_TIMER[prefix]) return;
    const [secsId, blockId] = PREFIX_TO_TIMER[prefix];
    const secsEl  = document.getElementById(secsId);
    const blockEl = document.getElementById(blockId);
    if (!secsEl) return;
    secsEl.textContent = remaining !== null && remaining !== undefined ? remaining : '--';
    if (blockEl) blockEl.classList.toggle('urgent', remaining !== null && remaining <= 5);
  }