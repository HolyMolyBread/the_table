  // ?? State ??????????????????????????????????????????????????????????????????
  let actionCooldown = false;  // 寃뚯엫 ?≪뀡 愿묓겢 諛⑹? (0.4珥?荑⑤떎??
  let ws            = null;
  let currentRoomId = '';
  let currentUserId = '';  // ?몄쬆 ???쒖떆紐?(?됰꽕???먮뒗 ?대찓??
  let currentUserEmail = '';  // ?몄쬆???대찓??二쇱냼 (?꾨줈??紐⑤떖 ?쒖떆??
  let currentToken  = '';  // Supabase JWT access_token (auth ?댄썑 ?ㅼ젙)
  let pendingJoin   = null; // { roomId } ??auth+connect ???먮룞 ?낆옣
  let pendingNickname = null; // ?뚯썝媛??吏곹썑 ?ㅼ젙???됰꽕??  let _signupNicknameChecked = false;   // ?뚯썝媛????以묐났?뺤씤 ?듦낵 ?щ?
  let _profileNicknameChecked = false;  // 留덉씠?섏씠吏 ??以묐났?뺤씤 ?듦낵 ?щ?
  let _onNicknameCheckDone = null;
  let reconnectTimer = null;
  let isIntentionalLeave = false;
  let currentMode = 'lobby'; // 'lobby' | 'room'

  // ?? GAME_CONFIG (SSOT: 濡쒕퉬 移대뱶 + 猷?紐⑤떖) ????????????????????????????????????
  const GAME_CONFIG = [
    { id: 'omok', type: 'board', icon: '??, title: '?ㅻぉ', desc: '15횞15 蹂대뱶?먯꽌 5紐⑹쓣 癒쇱? ?꾩꽦?섎㈃ ?밸━. ?묒뿉寃뚮뒗 ?띿궪쨌?띿궗쨌?λぉ 湲덉닔 ?곸슜', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '???ㅻぉ 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>15횞15 蹂대뱶?먯꽌 <strong>媛濡쑣룹꽭濡쑣룸?媛곸꽑?쇰줈 5紐?/strong>??癒쇱? ?꾩꽦?섎㈃ ?밸━?⑸땲??</li><li>2紐낆씠 ?낆옣?섎㈃ 臾댁옉?꾨줈 ???좉났)/諛??꾧났)??寃곗젙?⑸땲??</li></ul><h3>?뚯＜猷?(??湲덉닔)</h3><ul><li><strong>?띿궪(3-3)</strong>: ?쒖궪????諛⑺뼢 ?댁긽 ?숈떆???꾩꽦?섎뒗 怨?湲덉?</li><li><strong>?띿궗(4-4)</strong>: ??4紐??꾪삊)媛 ??諛⑺뼢 ?댁긽 ?숈떆???꾩꽦?섎뒗 怨?湲덉?</li><li><strong>?λぉ(6紐?)</strong>: 6媛??댁긽 ?곗냽 湲덉?</li><li>諛깆? 湲덉닔 ?놁쓬, 6紐??댁긽???밸━ ?몄젙</li></ul><h3>????대㉧</h3><ul><li>???대떦 <strong>15珥?/strong> ?쒗븳. 珥덇낵 ???쒓컙 珥덇낵 ?⑤같.</li></ul><h3>由щℓ移?/h3><ul><li>寃뚯엫 醫낅즺 ??<strong>?봽 ??????/strong> 踰꾪듉?쇰줈 由щℓ移?媛?? ??諛깆쓣 援먮??섏뿬 ??寃뚯엫 ?쒖옉.</li></ul><h3>?꾩쟻 湲곕줉</h3><ul><li>寃뚯엫 醫낅즺 ???뱀옄?먭쾶 <strong>1??/strong>, ?⑥옄?먭쾶 <strong>1??/strong>媛 ?꾩쟻??湲곕줉?⑸땲??</li></ul>` },
    { id: 'connect4', type: 'board', icon: '?뵶?윞', title: '4紐?(Connect 4)', desc: '6횞7 蹂대뱶?먯꽌 援먮?濡??댁쓣 ?좏깮???뚯쓣 ?⑥뼱?⑤젮 媛濡쑣룹꽭濡쑣룸?媛곸꽑 4媛쒕? 癒쇱? ?댁쑝硫??밸━!', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?뵶?윞 4紐?(Connect 4) 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>6??횞 7??蹂대뱶?먯꽌 <strong>媛濡쑣룹꽭濡쑣룸?媛곸꽑?쇰줈 4媛?/strong>瑜?癒쇱? ?댁쑝硫??밸━?⑸땲??</li><li>2紐낆씠 ?낆옣?섎㈃ ?뵶 鍮④컯(?좉났)怨??윞 ?몃옉(?꾧났)??寃곗젙?⑸땲??</li></ul><h3>吏꾪뻾 諛⑹떇</h3><ul><li>?대쭏????1~7)???섎굹 ?좏깮?섎㈃, ?뚯씠 <strong>以묐젰???섑빐 ?대떦 ?댁쓽 媛???꾨옒 鍮?移?/strong>?쇰줈 ?⑥뼱吏묐땲??</li><li>?댁씠 苑?李쇱쑝硫??대떦 ?댁쓣 ?좏깮?????놁뒿?덈떎.</li><li>42移몄씠 紐⑤몢 李④퀬 ?밸?媛 ?섏? ?딆쑝硫?<strong>臾댁듅遺</strong>?낅땲??</li></ul><h3>????대㉧</h3><ul><li>???대떦 <strong>15珥?/strong> ?쒗븳. 珥덇낵 ???쒓컙 珥덇낵 ?⑤같.</li></ul><h3>由щℓ移?/h3><ul><li>寃뚯엫 醫낅즺 ??<strong>?봽 ??????/strong> 踰꾪듉?쇰줈 由щℓ移?媛?? ?좏썑怨?鍮④컯/?몃옉) 援먮??섏뿬 ??寃뚯엫 ?쒖옉.</li></ul><h3>?꾩쟻 湲곕줉</h3><ul><li>?밸━ ?? <strong>1??/strong> 湲곕줉</li><li>臾댁듅遺: <strong>1臾?/strong> 湲곕줉</li><li>?⑤같 ?? <strong>1??/strong> 湲곕줉</li></ul>` },
    { id: 'tictactoe', type: 'board', icon: '狩뺚쓬', title: '?깊깮??(Tic-Tac-Toe)', desc: '3횞3 蹂대뱶?먯꽌 媛濡쑣룹꽭濡쑣룸?媛곸꽑 3紐⑹쓣 癒쇱? ?꾩꽦?섎㈃ ?밸━! ?밸Т??W/L/D) ?꾩쟻??湲곕줉?⑸땲??', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '狩??깊깮??猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>3횞3 蹂대뱶?먯꽌 <strong>媛濡쑣룹꽭濡쑣룸?媛곸꽑?쇰줈 3紐?/strong>??癒쇱? ?꾩꽦?섎㈃ ?밸━?⑸땲??</li><li>2紐낆씠 ?낆옣?섎㈃ O(?좉났)? X(?꾧났)媛 寃곗젙?⑸땲??</li></ul><h3>吏꾪뻾 諛⑹떇</h3><ul><li>O? X媛 踰덇컝??媛硫?鍮덉뭏???쒖떆?⑸땲??</li><li>9移몄씠 紐⑤몢 梨꾩썙吏怨??밸?媛 ?섏? ?딆쑝硫?<strong>臾댁듅遺</strong>?낅땲??</li></ul><h3>????대㉧</h3><ul><li>???대떦 <strong>15珥?/strong> ?쒗븳. 珥덇낵 ???쒓컙 珥덇낵 ?⑤같.</li></ul><h3>由щℓ移?/h3><ul><li>寃뚯엫 醫낅즺 ??<strong>?봽 ??????/strong> 踰꾪듉?쇰줈 由щℓ移?媛?? ?좏썑怨?O/X) 援먮??섏뿬 ??寃뚯엫 ?쒖옉.</li></ul><h3>?꾩쟻 湲곕줉</h3><ul><li>?밸━ ?? <strong>1??/strong> 湲곕줉</li><li>臾댁듅遺: <strong>1臾?/strong> 湲곕줉</li><li>?⑤같 ?? <strong>1??/strong> 湲곕줉</li></ul>` },
    { id: 'alkkagi', type: 'board', icon: '??, title: '?뚭퉴湲?(Alkkagi)', desc: '?묐룎怨?諛깅룎???뺢꺼 ?곷? ?뚯쓣 諛?대궡?몄슂! Matter.js 臾쇰━ ?붿쭊 湲곕컲 2?????', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '???뚭퉴湲?(Alkkagi) 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li><strong>2?????/strong>. ?묐룎 4媛?vs 諛깅룎 4媛?</li><li>?뚯쓣 ?뺢꺼 ?곷? ?뚯쓣 諛?대궡?몄슂!</li></ul><h3>議곗옉</h3><ul><li>??李⑤????뚯쓣 ?쒕옒洹명븯???밴릿 ???볦쑝硫??섏븘吏묐땲?? (?ν썑 援ы쁽)</li><li>?대씪?댁뼵??二쇰룄沅? 媛곸옄??釉뚮씪?곗?媛 臾쇰━ ?곗궛???섑뻾?⑸땲??</li></ul><h3>?밸━ 議곌굔</h3><ul><li>?곷? ?뚯쓣 紐⑤몢 蹂대뱶 諛뽰쑝濡?諛?대궡硫??밸━!</li></ul>` },
    { id: 'blackjack', type: 'card', icon: '?깗', title: '釉붾옓??, desc: '?쒕윭 AI瑜??곷?濡?21??媛??媛源앷쾶! ?밸Т??W/L/D) ?꾩쟻??湲곕줉?⑸땲??', badgeClass: 'pve', badgeText: 'PVE', ruleTitle: '?깗 釉붾옓??猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>?쒕윭 AI瑜??곷?濡??⑥쓽 ?⑹궛??<strong>21??媛??媛源앷쾶</strong> 留뚮뱶??寃뚯엫?낅땲??</li><li>21??珥덇낵(踰꾩뒪???섎㈃ 利됱떆 ?⑤같?⑸땲??</li></ul><h3>移대뱶 ?먯닔</h3><ul><li>?レ옄 移대뱶: ?대떦 ?レ옄</li><li>J, Q, K: 10??/li><li>A: 1 ?먮뒗 11 (踰꾩뒪??諛⑹? ?먮룞 議곗젙)</li></ul><h3>?쒕윭 洹쒖튃</h3><ul><li>?쒕윭???⑥쓽 ?⑹씠 <strong>16 ?댄븯</strong>?대㈃ 諛섎뱶??移대뱶瑜?異붽?濡?戮묒뒿?덈떎.</li><li>?쒕윭 移대뱶??1?μ씠 ?룸㈃?쇰줈 ?④꺼???덉쑝硫? Stand ??怨듦컻?⑸땲??</li></ul><h3>?꾩쟻 湲곕줉</h3><ul><li>?밸━ ?? <strong>1??/strong> 湲곕줉</li><li>臾댁듅遺(Push): <strong>1臾?/strong> 湲곕줉</li><li>?⑤같 / 踰꾩뒪?? <strong>1??/strong> 湲곕줉</li></ul>` },
    { id: 'holdem', type: 'card', icon: '?좑툘', title: '?띿궗?????, desc: '蹂?10媛쒕줈 ?쒖옉! 泥댄겕(狩?-1) ?먮뒗 ?대뱶. 而ㅻ??덊떚 移대뱶 5?μ쑝濡?議깅낫 ?寃? ?덈컲 ?뚯궛 ???앹〈???밸━!', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?좑툘 ?띿궗?????猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>理쒕? 4?? 媛??뚮젅?댁뼱??<strong>蹂?狩?횞10媛?/strong>濡??쒖옉?⑸땲??</li><li>?꾪넻?곸씤 移?踰좏똿(?덉씠利??ъ씤) ?놁씠, 罹먯＜?쇳븳 <strong>蹂??쒕컮?대쾶 猷?/strong>???곸슜?⑸땲??</li></ul><h3>?섏씠利?/h3><ul><li><strong>?꾨━?뚮엻</strong>: 媛쒖씤 移대뱶 2??諛곕텇 ??泥댄겕/?대뱶</li><li><strong>?뚮엻</strong>: 而ㅻ??덊떚 移대뱶 3??怨듦컻 ??泥댄겕/?대뱶</li><li><strong>??/strong>: 而ㅻ??덊떚 移대뱶 +1????泥댄겕/?대뱶</li><li><strong>由щ쾭</strong>: 而ㅻ??덊떚 移대뱶 +1????泥댄겕/?대뱶</li><li><strong>?쇰떎??/strong>: ?앹〈?먮뱾??7??媛쒖씤 2??怨듭쑀 5???쇰줈 議깅낫 ?먯젙, 理쒓퀬 議깅낫媛 ???띾뱷</li></ul><h3>?≪뀡</h3><ul><li>??<strong>泥댄겕</strong>: ?잛뿉 蹂?1媛?吏遺덊븯怨??ㅼ쓬 ?섏씠利?吏꾪뻾. (蹂?0媛쒕㈃ 臾대즺 泥댄겕)</li><li>?뤂截?<strong>?대뱶</strong>: ?대쾲 ?쇱슫???ш린. (?대? ??蹂꾩? ?뚮젮諛쏆? 紐삵븿)</li></ul><h3>?쇰떎??諛?議깅낫</h3><ul><li>?숈젏 ???잛쓣 n?깅텇, ?섎㉧吏???ㅼ쓬 ?쇱슫?쒕줈 ?댁썡</li></ul><h3>?뱥 ?ъ빱 議깅낫 ?쒖꽌</h3><p>濡쒗떚??&gt; ?ㅽ듃?덉씠?명뵆?ъ떆 &gt; ?ъ뭅??&gt; ??섏슦??&gt; ?뚮윭??&gt; ?ㅽ듃?덉씠??&gt; ?몃━??&gt; ?ы럹??&gt; ?먰럹??&gt; ?섏씠移대뱶</p><h3>留ㅼ튂 醫낅즺</h3><ul><li><strong>?뚯궛??????ceil(?꾩껜 ?좎? ??/ 2)</strong>?대㈃ 留ㅼ튂 醫낅즺.</li><li>?앹〈??蹂?蹂댁쑀) ??<strong>1??/strong>, ?뚯궛????<strong>1??/strong> ?꾩쟻 湲곕줉.</li></ul>` },
    { id: 'sevenpoker', type: 'card', icon: '?ｏ툘', title: '?몃툙 ?ъ빱', desc: '3~7援?遺꾨같! 媛곸옄 7?? ?덈뱺 移대뱶媛 ?덈뒗 蹂??쒕컮?대쾶. 泥댄겕(狩?-1) ?먮뒗 ?대뱶濡??밸?!', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?깗 ?몃툙 ?ъ빱 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>理쒕? 4?? 媛??뚮젅?댁뼱??<strong>蹂?狩?횞10媛?/strong>濡??쒖옉?⑸땲??</li><li>而ㅻ??덊떚 移대뱶 ?놁씠 <strong>媛곸옄 7??/strong>??諛쏅뒗 3~7援?遺꾨같 諛⑹떇?낅땲??</li></ul><h3>移대뱶 遺꾨같 (3~7援?</h3><ul><li><strong>3援?/strong>: 3??遺꾨같 (1~2踰덉㎏ 移대뱶 ?덈뱺, 3踰덉㎏ 怨듦컻)</li><li><strong>4援?/strong>: 4踰덉㎏ 移대뱶 怨듦컻</li><li><strong>5援?/strong>: 5踰덉㎏ 移대뱶 怨듦컻</li><li><strong>6援?/strong>: 6踰덉㎏ 移대뱶 怨듦컻</li><li><strong>7援?/strong>: 7踰덉㎏ 移대뱶 ?덈뱺 (?쇰떎???꾧퉴吏 鍮꾧났媛?</li></ul><h3>?≪뀡</h3><ul><li>??<strong>泥댄겕</strong>: ?잛뿉 蹂?1媛?吏遺?(蹂?0媛쒕㈃ 臾대즺 泥댄겕)</li><li>?뤂截?<strong>?대뱶</strong>: ?대쾲 ?쇱슫???ш린</li></ul><h3>?쇰떎??諛?議깅낫</h3><ul><li>7??以?踰좎뒪??5??議깅낫濡??밸?.</li><li>?숈젏 ????遺꾪븷, ?섎㉧吏???ㅼ쓬 ?쇱슫???댁썡.</li></ul><h3>?뱥 ?ъ빱 議깅낫 ?쒖꽌</h3><p>濡쒗떚??&gt; ?ㅽ듃?덉씠?명뵆?ъ떆 &gt; ?ъ뭅??&gt; ??섏슦??&gt; ?뚮윭??&gt; ?ㅽ듃?덉씠??&gt; ?몃━??&gt; ?ы럹??&gt; ?먰럹??&gt; ?섏씠移대뱶</p><h3>留ㅼ튂 醫낅즺</h3><ul><li>?뚯궛??蹂?0媛? ???꾩껜 ?좎? ??2 ?대㈃ 留ㅼ튂 醫낅즺.</li><li>?앹〈????1?? ?뚯궛????1???꾩쟻 湲곕줉.</li></ul>` },
    { id: 'indian', type: 'card', icon: '?쩆', title: '?몃뵒???ъ빱', desc: '?곷? 移대뱶??蹂댁씠吏留???移대뱶????蹂댁뿬?? ?섑듃 10媛쒕줈 ?쒖옉, ?쒕컮?대쾶 ?밸?!', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?깗 ?몃뵒???ъ빱 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>?먯떊??移대뱶??蹂????녾퀬, <strong>?곷?諛⑹쓽 移대뱶留?蹂????덉뒿?덈떎.</strong></li><li>媛??뚮젅?댁뼱??<strong>?ㅿ툘횞10 ?섑듃(?멸쾶??泥대젰)</strong>濡??쒖옉?⑸땲?? ?섑듃???쇱슫??吏꾪뻾???꾪븳 ?앸챸?μ씠硫? 0???섎㈃ 留ㅼ튂?먯꽌 ?덈씫?⑸땲??</li></ul><h3>?쇱슫??吏꾪뻾</h3><ul><li>留??쇱슫??媛??뚮젅?댁뼱??移대뱶瑜?1??諛쏆뒿?덈떎. ?곷? 移대뱶瑜?蹂닿퀬 ?먮떒?섏꽭??</li><li>?좉났??癒쇱? ?좏깮?⑸땲??</li><li>&nbsp;&nbsp;?뤂截?<strong>?ш린</strong>: ?좉났 ?섑듃 -1. ?쇱슫??醫낅즺.</li><li>&nbsp;&nbsp;?뷂툘 <strong>?밸?</strong>: ?꾧났?먭쾶 李⑤?媛 ?섏뼱媛묐땲??</li><li>?꾧났???좏깮?⑸땲??</li><li>&nbsp;&nbsp;?뤂截?<strong>?ш린</strong>: ?꾧났 ?섑듃 -1. ?쇱슫??醫낅즺.</li><li>&nbsp;&nbsp;?뷂툘 <strong>?밸?(肄?</strong>: 移대뱶瑜?怨듦컻?섏뿬 ?뱁뙣瑜??먯젙?⑸땲??</li></ul><h3>?쇱슫?????섑듃 利앷컧 (?멸쾶??泥대젰)</h3><ul><li>?ш린 ?? ?대떦 ?뚮젅?댁뼱 ?섑듃 -1.</li><li>?밸?(肄? ?? ?뱀옄 ?섑듃 +2, ?⑥옄 ?섑듃 -2.</li><li>?レ옄(2 &lt; 3 &lt; ??&lt; K &lt; A)媛 ?믪? 履??밸━. ?숈젏 ??臾몄뼇(??&lt; ??&lt; ??&lt; ???쇰줈 寃곗젙.</li></ul><h3>????대㉧</h3><ul><li>???대떦 <strong>30珥?/strong> ?쒗븳. 珥덇낵 ??<strong>?ш린</strong> 泥섎━(?섑듃 -1).</li></ul><h3>?쇱슫?쒕쭏???좏썑怨?援먯껜</h3><ul><li>留??쇱슫???좉났怨??꾧났??援먮??⑸땲??</li></ul><h3>由щℓ移?/h3><ul><li>寃뚯엫 ?꾩쟾 醫낅즺(?꾧뎔媛 ?섑듃 0媛? ??<strong>?봽 ??????/strong> 踰꾪듉?쇰줈 由щℓ移?媛?? ?묒そ ?섑듃瑜?10媛쒕줈 由ъ뀑?섏뿬 ??寃뚯엫 ?쒖옉.</li></ul><h3>理쒖쥌 ?꾩쟻 湲곕줉</h3><ul><li>?꾧뎔媛???섑듃媛 0???섏뼱 留ㅼ튂媛 ?꾩쟾??醫낅즺?섎㈃, 理쒖쥌 ?앹〈?먯뿉寃?<strong>1??/strong>, ?덈씫?먯뿉寃?<strong>1??/strong>媛 ?쒕쾭 ?꾩쟻??湲곕줉?⑸땲??</li></ul>` },
    { id: 'thief', type: 'card', icon: '?깗', title: '?꾨몣?↔린 (Thief)', desc: '53??52+議곗빱) 遺꾨같 ???섏뼱 ?쒓굅. ?ㅼ쓬 ?뚮젅?댁뼱 ?⑥뿉??移대뱶 1??戮묎린. ??0?μ씠硫??덉텧! 議곗빱留??⑥쑝硫??⑤같.', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?깗 ?꾨몣?↔린 (Thief) 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>2~4?? 52??議곗빱 1??珥?<strong>53??/strong>??遺꾨같?⑸땲??</li><li>遺꾨같 吏곹썑 媛숈? ?レ옄??移대뱶(?섏뼱)瑜??먮룞?쇰줈 ?쒓굅?⑸땲??</li></ul><h3>??吏꾪뻾</h3><ul><li>??李⑤?????<strong>?ㅼ쓬 ?앹〈 ?뚮젅?댁뼱</strong>???⑥뿉??移대뱶 1?μ쓣 臾댁옉?꾨줈 戮묒븘 ?듬땲??</li><li>戮묒? 吏곹썑 ???⑥뿉 媛숈? ?レ옄媛 ?앷린硫?利됱떆 ?섏뼱濡?踰꾨┰?덈떎.</li></ul><h3>?뱁뙣</h3><ul><li>?④? 0?μ씠 ?섎㈃ <strong>?덉텧(Win)</strong> ???댁뿉???쒖쇅?⑸땲??</li><li>理쒖쥌?곸쑝濡?<strong>議곗빱 1?λ쭔 ?ㅺ퀬 ?⑥? 1紐?/strong>???⑤같(Lose)?섎ŉ 寃뚯엫 醫낅즺.</li></ul>` },
    { id: 'onecard', type: 'card', icon: '?깗', title: '?먯뭅??(One Card)', desc: '諛붾떏 移대뱶? 臾몄뼇/?レ옄 ?쇱튂 移대뱶留??????덉쓬. J=?ㅽ궢, Q=諛⑺뼢諛섏쟾. ??0?μ씠硫??밸━!', badgeClass: 'pvp', badgeText: 'PVP', ruleTitle: '?깗 ?먯뭅??(One Card) 猷?, ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li>2~4?? 54????52??+ ?묐갚 議곗빱 + 而щ윭 議곗빱)?먯꽌 媛?<strong>7?μ뵫</strong> 遺꾨같.</li><li>?깆씠 鍮꾨㈃ 踰꾨┛ 移대뱶瑜??뷀뵆?섏뿬 ?ы솢?⑺빀?덈떎.</li></ul><h3>??吏꾪뻾</h3><ul><li>諛붾떏 移대뱶? <strong>臾몄뼇</strong> ?먮뒗 <strong>?レ옄</strong>媛 ?쇱튂?섎뒗 移대뱶留??????덉뒿?덈떎.</li><li>議곗빱???몄젣???????덉뒿?덈떎.</li><li>??移대뱶媛 ?놁쑝硫??깆뿉???쒕줈?고빀?덈떎.</li></ul><h3>怨듦꺽 移대뱶</h3><ul><li><strong>A</strong>: +3??/li><li><strong>?묐갚 議곗빱(B)</strong>: +5??/li><li><strong>而щ윭 議곗빱(C)</strong>: +7??/li><li>怨듦꺽??諛쏆쑝硫?諛⑹뼱 移대뱶濡?留됯굅?? ?쒕줈?곕? ?좏깮???⑤꼸?곕쭔??諛쏆뒿?덈떎.</li><li>諛⑹뼱: A?묨/議곗빱, ?묐갚 議곗빱?믪뺄??議곗빱留? 而щ윭 議곗빱??諛⑹뼱 遺덇?.</li></ul><h3>?뱀닔 移대뱶</h3><ul><li><strong>J</strong>: ?ㅼ쓬 ?щ엺 ???ㅽ궢</li><li><strong>Q</strong>: ??吏꾪뻾 諛⑺뼢 諛섏쟾</li><li><strong>K</strong>: ??踰??? (?닿? ?ㅼ떆 移대뱶瑜???</li><li><strong>7</strong>: ?ㅼ쓬 臾몄뼇(?졻솯?╈솭) 媛뺤젣 蹂寃?/li></ul><h3>?먯뭅??肄?/h3><ul><li>?먰뙣媛 1?μ씠 ?섎㈃ <strong>?먯뭅??</strong> 踰꾪듉???쒖꽦?붾맗?덈떎.</li><li>蹂몄씤??癒쇱? ?꾨Ⅴ硫??덉쟾?댁?怨? ??몄씠 癒쇱? ?꾨Ⅴ硫?1?μ씤 ?좎?媛 踰뚯튃 1???쒕줈??</li></ul><h3>?뚯궛 & ?뱁뙣</h3><ul><li>?먰뙣媛 <strong>20??珥덇낵</strong> ??利됱떆 ?뚯궛(?⑤같).</li><li>?④? 0?μ씠 ?섎㈃ <strong>?밸━</strong>, ?섎㉧吏 <strong>?⑤같</strong>.</li></ul>` },
    { id: 'mahjong', type: 'card', icon: '??, title: '留덉옉 (Mahjong)', desc: '4???꾩슜. 136????遺꾨같 ??易붾え쨌??? 14?μ씠 ?섎㈃ ?⑤? 踰꾨━?몄슂. (Phase 1)', badgeClass: 'pvp', badgeText: '4??, ruleTitle: '??留덉옉 (Mahjong) 猷???Phase 1', ruleHtml: `<h3>寃뚯엫 媛쒖슂</h3><ul><li><strong>4???꾩슜</strong>. 136???섑뙣 108??+ ?먰뙣 28?????ъ슜?⑸땲??</li><li>?꾩썝 Ready ??寃뚯엫???쒖옉?⑸땲??</li></ul><h3>??遺꾨같</h3><ul><li>4紐낆뿉寃?媛곴컖 <strong>13??/strong>??遺꾨같?⑸땲??</li><li>??移?遺???댁씠 ?쒖옉?섎ŉ, ???쒖옉 ??<strong>易붾え</strong> 1?μ쓣 戮묒븘 14?μ씠 ?⑸땲??</li></ul><h3>???(Discard)</h3><ul><li>??李⑤???14?μ쓽 ?먰뙣 以?1?μ쓣 ?좏깮??踰꾨┰?덈떎.</li><li>踰꾨┛ ???ㅼ쓬 ?뚮젅?댁뼱濡??댁씠 ?섏뼱媛怨? 洹??뚮젅?댁뼱媛 ?먮룞?쇰줈 1??易붾え?⑸땲??</li></ul><h3>Phase 1 踰붿쐞</h3><ul><li>移???源?諛???怨꾩궛? ?꾩쭅 援ы쁽?섏? ?딆븯?듬땲??</li></ul>` }
  ];

  function renderLobbyGames() {
    const boardEl = document.getElementById('board-games');
    const cardEl = document.getElementById('card-games');
    if (!boardEl || !cardEl) return;
    boardEl.innerHTML = '';
    cardEl.innerHTML = '';
    GAME_CONFIG.forEach(g => {
      const html = `<div class="game-card">
        <div class="card-icon">${g.icon}</div>
        <div class="card-title">${escapeHTML(g.title)}</div>
        <div class="card-desc">${escapeHTML(g.desc)}</div>
        <div class="card-badge ${g.badgeClass}">${escapeHTML(g.badgeText)}</div>
        <div class="card-actions-bottom">
          <div class="card-top-actions">
            <button class="card-btn-rule" onclick="showRules('${g.id}')">?뱰 猷?/button>
            <button class="card-btn-create" onclick="createRoom('${g.id}')">??諛?留뚮뱾湲?/button>
          </div>
          <div class="card-join-row">
            <input type="text" class="card-code-input" id="code-input-${g.id}"
                   maxlength="6" placeholder="肄붾뱶 6?먮━"
                   autocomplete="off" autocorrect="off" autocapitalize="characters" spellcheck="false"
                   onkeydown="if(event.key==='Enter') joinWithCode('${g.id}','code-input-${g.id}')" />
            <button class="card-btn-join-code" onclick="joinWithCode('${g.id}','code-input-${g.id}')">?낆옣</button>
          </div>
        </div>
      </div>`;
      if (g.type === 'board') boardEl.insertAdjacentHTML('beforeend', html);
      else cardEl.insertAdjacentHTML('beforeend', html);
    });
  }

  // Indian Poker 移대뱶 ?뚮뜑留?理쒖쟻??(蹂寃??쒖뿉留??낅뜲?댄듃)
  let lastIndianOppCard = '';
  let lastIndianMyCard  = '';

  // Gomoku
  let gomokuMyColor    = 0;
  let gomokuTurnUserId = '';
  let gomokuColorMap   = {};
  let gomokuBoardReady = false;
  let gomokuEnded      = false; // 寃뚯엫 醫낅즺 ??由щℓ移?踰꾪듉 ?쒖떆 ?щ?
  let gomokuPrevBoard  = null;  // diffing???댁쟾 蹂대뱶 (15x15)
  let tttBoardReady    = false;
  let tttPrevBoard     = null;  // diffing???댁쟾 蹂대뱶 (3x3)
  let c4BoardReady     = false;
  let c4PrevBoard      = null;  // diffing???댁쟾 蹂대뱶 (6x7)

  const STAR_POINTS = new Set(
    [3,7,11].flatMap(r => [3,7,11].map(c => `${r},${c}`))
  );

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

  // ?? 諛깃렇?쇱슫????蹂듦? ???곌껐 ?딄? 泥섎━ ?????????????????????????????????????
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      // 濡쒕퉬 ?붾㈃(room???꾨땺 ???먯꽌留??먮룞 ?ъ뿰寃??덉슜
      if (currentMode !== 'room' && (!ws || ws.readyState !== WebSocket.OPEN)) {
        if (currentToken) connect();
        else window.location.reload();
      }
    }
  });

  // ?? Init ??????????????????????????????????????????????????????????????????
  (function init() {
    renderLobbyGames();
    setConnectionState(false);
    // ?대? 濡쒓렇?몃맂 ?몄뀡???덉쑝硫??먮룞 蹂듦뎄
    supabaseClient.auth.getSession().then(({ data: { session } }) => {
      if (session) {
        currentToken     = session.access_token;
        currentUserEmail = session.user.email;
        currentUserId    = session.user.email;  // auth_ok?먯꽌 ?됰꽕?꾩쑝濡?媛깆떊??        showLoggedIn(session.user.email);
        connect();
      }
    });
  })();

  // ?? Supabase Auth ?⑥닔????????????????????????????????????????????????????
  // ?? Auth 紐⑤뱶 ?꾪솚 ?????????????????????????????????????????????????????????
  let _authMode = 'login'; // 'login' | 'signup'

  function toggleAuthMode(mode) {
    _authMode = mode;
    const isSignup = mode === 'signup';
    document.getElementById('auth-panel-title').textContent  = isSignup ? '?뱷 ?뚯썝媛?? : '?뵍 濡쒓렇??;
    document.getElementById('auth-confirm-group').style.display = isSignup ? 'block' : 'none';
    document.getElementById('auth-nickname-group').style.display = isSignup ? 'block' : 'none';
    document.getElementById('auth-nickname').value = '';
    document.getElementById('auth-nickname-status').textContent = '';
    document.getElementById('auth-login-btn-row').style.display   = isSignup ? 'none' : '';
    document.getElementById('auth-login-switch').style.display    = isSignup ? 'none' : '';
    document.getElementById('auth-signup-btn-row').style.display  = isSignup ? '' : 'none';
    document.getElementById('auth-signup-switch').style.display   = isSignup ? '' : 'none';
    document.getElementById('auth-password').placeholder          = isSignup ? '鍮꾨?踰덊샇 (6???댁긽)' : '鍮꾨?踰덊샇';
    document.getElementById('auth-password-confirm').value        = '';
    _signupNicknameChecked = false;
    document.getElementById('btn-auth-signup').disabled = true;
    setAuthStatus('');
  }

  /** ?됰꽕???낅젰 ??以묐났?뺤씤 媛뺤젣 由ъ뀑 ???뚯썝媛??留덉씠?섏씠吏 怨듯넻 */
  function resetNicknameCheck(type) {
    if (type === 'signup') {
      _signupNicknameChecked = false;
      document.getElementById('btn-auth-signup').disabled = true;
      const statusEl = document.getElementById('auth-nickname-status');
      statusEl.textContent = '?좑툘 以묐났?뺤씤???댁＜?몄슂';
      statusEl.style.color = 'var(--warning)';
    } else if (type === 'profile') {
      _profileNicknameChecked = false;
      document.getElementById('btn-profile-apply').disabled = true;
      const statusEl = document.getElementById('profile-nickname-status');
      statusEl.textContent = '?좑툘 以묐났?뺤씤???댁＜?몄슂';
      statusEl.style.color = 'var(--warning)';
    }
  }

  function onAuthKeyDown(e) {
    if (e.key !== 'Enter') return;
    if (_authMode === 'signup') authSignup();
    else authLogin();
  }

  /** ?뚯썝媛?????됰꽕??以묐났?뺤씤 ??Supabase 吏곸젒 議고쉶 (WS 誘몄뿰寃??곹깭) */
  async function checkNickname() {
    const nickname = document.getElementById('auth-nickname').value.trim();
    const statusEl = document.getElementById('auth-nickname-status');
    if (!nickname) { statusEl.textContent = '?됰꽕?꾩쓣 ?낅젰?섏꽭??'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname.length < 2 || nickname.length > 20) { statusEl.textContent = '?됰꽕?꾩? 2~20?먮줈 ?낅젰?섏꽭??'; statusEl.style.color = 'var(--danger)'; return; }
    statusEl.textContent = '?뺤씤 以?..'; statusEl.style.color = 'var(--text-secondary)';
    try {
      const { data, error } = await supabaseClient.from('profiles').select('id').eq('username', nickname).limit(1);
      if (error) throw error;
      const available = !data || data.length === 0;
      _signupNicknameChecked = available;
      document.getElementById('btn-auth-signup').disabled = !available;
      if (available) { statusEl.textContent = '???ъ슜 媛?ν븳 ?됰꽕?꾩엯?덈떎.'; statusEl.style.color = 'var(--success)'; }
      else { statusEl.textContent = '???대? ?ъ슜 以묒씤 ?됰꽕?꾩엯?덈떎.'; statusEl.style.color = 'var(--danger)'; }
    } catch (e) {
      statusEl.textContent = '?뺤씤 以??ㅻ쪟媛 諛쒖깮?덉뒿?덈떎.'; statusEl.style.color = 'var(--danger)';
    }
  }

  function sendSetNickname(nickname) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendRaw(JSON.stringify({ action: 'set_nickname', payload: { nickname } }));
  }

  function openProfileModal() {
    if (currentRoomId) {
      alert('寃뚯엫 吏꾪뻾 以묒뿉???꾨줈?꾩쓣 蹂寃쏀븷 ???놁뒿?덈떎.');
      return;
    }
    document.getElementById('profile-email').textContent = currentUserEmail || currentUserId || '??;
    document.getElementById('profile-current-nickname').textContent = currentUserId || '??;
    document.getElementById('profile-new-nickname').value = currentUserId || '';
    _profileNicknameChecked = false;
    document.getElementById('btn-profile-apply').disabled = true;
    document.getElementById('profile-nickname-status').textContent = '?좑툘 以묐났?뺤씤???댁＜?몄슂';
    document.getElementById('profile-nickname-status').style.color = 'var(--warning)';
    document.getElementById('profile-modal').classList.add('show');
  }
  function closeProfileModal() {
    document.getElementById('profile-modal').classList.remove('show');
  }
  /** 留덉씠?섏씠吏 ?됰꽕??以묐났?뺤씤 ??Supabase 吏곸젒 議고쉶 */
  async function checkNicknameProfile() {
    const nickname = document.getElementById('profile-new-nickname').value.trim();
    const statusEl = document.getElementById('profile-nickname-status');
    if (!nickname) { statusEl.textContent = '?됰꽕?꾩쓣 ?낅젰?섏꽭??'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname.length < 2 || nickname.length > 20) { statusEl.textContent = '?됰꽕?꾩? 2~20?먮줈 ?낅젰?섏꽭??'; statusEl.style.color = 'var(--danger)'; return; }
    if (nickname === currentUserId) {
      _profileNicknameChecked = true;
      document.getElementById('btn-profile-apply').disabled = false;
      statusEl.textContent = '???꾩옱 ?됰꽕?꾧낵 ?숈씪?⑸땲??'; statusEl.style.color = 'var(--success)';
      return;
    }
    statusEl.textContent = '?뺤씤 以?..'; statusEl.style.color = 'var(--text-secondary)';
    try {
      const { data, error } = await supabaseClient.from('profiles').select('id').eq('username', nickname).limit(1);
      if (error) throw error;
      const available = !data || data.length === 0;
      _profileNicknameChecked = available;
      document.getElementById('btn-profile-apply').disabled = !available;
      if (available) { statusEl.textContent = '???ъ슜 媛?ν븳 ?됰꽕?꾩엯?덈떎.'; statusEl.style.color = 'var(--success)'; }
      else { statusEl.textContent = '???대? ?ъ슜 以묒씤 ?됰꽕?꾩엯?덈떎.'; statusEl.style.color = 'var(--danger)'; }
    } catch (e) {
      statusEl.textContent = '?뺤씤 以??ㅻ쪟媛 諛쒖깮?덉뒿?덈떎.'; statusEl.style.color = 'var(--danger)';
    }
  }
  function saveProfileNickname() {
    if (!_profileNicknameChecked) { showToast('以묐났?뺤씤??癒쇱? ?댁＜?몄슂.', 'error'); return; }
    const nickname = document.getElementById('profile-new-nickname').value.trim();
    if (!nickname || nickname.length < 2 || nickname.length > 20) {
      showToast('?됰꽕?꾩? 2~20?먮줈 ?낅젰?섏꽭??', 'error'); return;
    }
    if (!ws || ws.readyState !== WebSocket.OPEN) { showToast('?곌껐???딆뼱議뚯뒿?덈떎.', 'error'); return; }
    sendRaw(JSON.stringify({ action: 'set_nickname', payload: { nickname } }));
    currentUserId = nickname;
    closeProfileModal();
    showToast('?됰꽕?꾩씠 蹂寃쎈릺?덉뒿?덈떎.', 'info');
  }

  function requestOpponentRecord(userId) {
    if (!userId || userId === currentUserId) return;
    if (!ws || ws.readyState !== WebSocket.OPEN) { showToast('?곌껐???딆뼱議뚯뒿?덈떎.', 'error'); return; }
    sendRaw(JSON.stringify({ action: 'get_user_record', payload: { userId } }));
  }
  function showOpponentRecordModal(userId, records) {
    const fmt = r => r ? `${r.wins}??${r.losses}??${r.draws}臾? : '0??0??0臾?;
    const games = ['omok', 'tictactoe', 'connect4', 'holdem', 'sevenpoker', 'indian', 'blackjack', 'onecard', 'thief', 'mahjong', 'alkkagi'];
    const labels = { total: '?꾩껜', omok: '???ㅻぉ', tictactoe: '狩뺚쓬 ?깊깮??, connect4: '?뵶?윞 4紐?, holdem: '?좑툘 ?띿궗?????, sevenpoker: '?깗 ?몃툙 ?ъ빱', indian: '?깗 ?몃뵒???ъ빱', blackjack: '?깗 釉붾옓??, onecard: '?깗 ?먯뭅??, thief: '?깗 ?꾨몣?↔린', mahjong: '??留덉옉', alkkagi: '???뚭퉴湲? };
    let html = '';
    html += `<div class="opponent-record-row"><span class="opponent-record-label">${labels.total}</span><span class="opponent-record-val">${fmt(records && records.total)}</span></div>`;
    for (const key of games) {
      const label = labels[key];
      if (label) {
        const r = records && records[key];
        html += `<div class="opponent-record-row"><span class="opponent-record-label">${label}</span><span class="opponent-record-val">${fmt(r)}</span></div>`;
      }
    }
    document.getElementById('opponent-record-title').textContent = `${userId} ?꾩쟻`;
    document.getElementById('opponent-record-content').innerHTML = html || '<div>?꾩쟻 ?놁쓬</div>';
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
    if (!email || !password) { setAuthStatus('?대찓?쇨낵 鍮꾨?踰덊샇瑜??낅젰?섏꽭??', 'error'); return; }
    setAuthStatus('濡쒓렇??以?..', 'loading');
    const { data, error } = await supabaseClient.auth.signInWithPassword({ email, password });
    if (error) { setAuthStatus(error.message, 'error'); return; }
    currentToken     = data.session.access_token;
    currentUserEmail = data.user.email;
    currentUserId    = data.user.email;  // auth_ok?먯꽌 ?됰꽕?꾩쑝濡?媛깆떊??    showLoggedIn(data.user.email);
    connect();
  }

  async function authSignup() {
    const email    = document.getElementById('auth-email').value.trim();
    const password = document.getElementById('auth-password').value;
    const confirm  = document.getElementById('auth-password-confirm').value;
    const nickname = document.getElementById('auth-nickname').value.trim();
    if (!email || !password) { setAuthStatus('?대찓?쇨낵 鍮꾨?踰덊샇瑜??낅젰?섏꽭??', 'error'); return; }
    if (password.length < 6)  { setAuthStatus('鍮꾨?踰덊샇??6???댁긽?댁뼱???⑸땲??', 'error'); return; }
    if (password !== confirm)  { setAuthStatus('鍮꾨?踰덊샇媛 ?쇱튂?섏? ?딆뒿?덈떎.', 'error'); return; }
    if (!nickname || nickname.length < 2 || nickname.length > 20) {
      setAuthStatus('?됰꽕?꾩? 2~20?먮줈 ?낅젰?섏꽭??', 'error'); return;
    }
    if (!_signupNicknameChecked) {
      setAuthStatus('?됰꽕??以묐났?뺤씤???댁＜?몄슂.', 'error'); return;
    }
    setAuthStatus('媛??以?..', 'loading');
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
      setAuthStatus('?벁 媛???뺤씤 ?대찓?쇱쓣 諛쒖넚?덉뒿?덈떎. ?대찓?쇱쓣 ?뺤씤??二쇱꽭??', 'loading');
    }
  }

  async function authLogout() {
    await supabaseClient.auth.signOut();
    currentToken  = '';
    currentUserId = '';
    if (ws) ws.close();
    showLoggedOut();
    setRoomState('', '');
    showToast('濡쒓렇?꾩썐 ?섏뿀?듬땲??', 'info');
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

  // ?? Connection State ???????????????????????????????????????????????????????
  function setConnectionState(connected) {
    const dot  = document.getElementById('conn-dot');
    const text = document.getElementById('conn-text');
    dot.className  = connected ? 'connected' : '';
    text.textContent = connected ? '?곌껐?? : '?곌껐 ?딄?';

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

  // ?? Room State (lobby ??room transition) ???????????????????????????????????
  function setRoomState(userId, roomId) {
    // currentUserId??auth ???ㅼ젙?? 諛??댁옣 ?쒖뿉???좎?.
    if (userId) currentUserId = userId;
    currentRoomId = roomId;
    currentMode = roomId ? 'room' : 'lobby';

    if (roomId) {
      // ?? Enter room ??
      document.getElementById('lobby-view').style.display = 'none';
      document.getElementById('room-view').classList.add('active');
      document.getElementById('btn-leave').style.display = '';
      document.getElementById('chat-room-badge').textContent = roomId;
      document.getElementById('chat-input').disabled      = false;
      document.getElementById('btn-send-chat').disabled   = false;

      // Board title
      const titleEl = document.getElementById('game-area-title');
      if      (roomId.startsWith('omok'))      titleEl.textContent = '???ㅻぉ';
      else if (roomId.startsWith('blackjack')) titleEl.textContent = '?깗 釉붾옓??;
      else if (roomId.startsWith('tictactoe')) titleEl.textContent = '狩뺚쓬 ?깊깮??;
      else if (roomId.startsWith('connect4'))  titleEl.textContent = '?뵶?윞 4紐?;
      else if (roomId.startsWith('indian'))    titleEl.textContent = '?깗 ?몃뵒???ъ빱';
      else if (roomId.startsWith('thief'))     titleEl.textContent = '?깗 ?꾨몣?↔린';
      else if (roomId.startsWith('onecard'))   titleEl.textContent = '?깗 ?먯뭅??;
      else if (roomId.startsWith('mahjong'))   titleEl.textContent = '??留덉옉';
      else if (roomId.startsWith('alkkagi'))   titleEl.textContent = '???뚭퉴湲?;
      else                                     titleEl.textContent = roomId;

      // Room code badge: extract last segment after last underscore
      const codeParts = roomId.split('_');
      const roomCode  = codeParts[codeParts.length - 1].toUpperCase();
      document.getElementById('room-code-text').textContent = roomCode;
      document.getElementById('room-code-badge').classList.add('visible');

      // ?멸쾶??猷?踰꾪듉: omok / blackjack / tictactoe / connect4 / indian / holdem / sevenpoker / thief / onecard 諛⑹뿉???쒖떆
      const rulesBtn = document.getElementById('btn-ingame-rules');
      rulesBtn.style.display = (roomId.startsWith('omok') || roomId.startsWith('blackjack') || roomId.startsWith('tictactoe') || roomId.startsWith('connect4') || roomId.startsWith('indian') || roomId.startsWith('holdem') || roomId.startsWith('sevenpoker') || roomId.startsWith('thief') || roomId.startsWith('onecard') || roomId.startsWith('mahjong') || roomId.startsWith('alkkagi')) ? '' : 'none';
      // 遊?異붽? 踰꾪듉: ?ㅻぉ, 4紐? ?깊깮?? ?몃뵒???ъ빱, ??? ?몃툙 ?ъ빱, ?꾨몣?↔린, ?먯뭅??諛⑹뿉???쒖떆 (blackjack ?쒖쇅)
      const addBotBtn = document.getElementById('btn-add-bot');
      addBotBtn.style.display = (roomId.startsWith('omok') || roomId.startsWith('connect4') || roomId.startsWith('tictactoe') || roomId.startsWith('indian') || roomId.startsWith('holdem') || roomId.startsWith('sevenpoker') || roomId.startsWith('thief') || roomId.startsWith('onecard') || roomId.startsWith('mahjong') || roomId.startsWith('alkkagi')) ? '' : 'none';

      // Sync debug inputs
      if (inputUserId) inputUserId.value = userId;
      if (inputRoomId) inputRoomId.value = roomId;

      // Ready ?곸뿭: blackjack???꾨땶 PVP 寃뚯엫?먯꽌???쒖떆
      const readyArea = document.getElementById('ready-area');
      const btnReady = document.getElementById('btn-ready');
      const readyCountEl = document.getElementById('ready-count');
      if (!roomId.startsWith('blackjack')) {
        readyArea.style.display = 'flex';
        if (btnReady) btnReady.disabled = false;
        if (readyCountEl) readyCountEl.textContent = '0/0';
      } else {
        readyArea.style.display = 'none';
      }

    } else {
      // ?? Leave room / show lobby ??
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

      // Reset game boards
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
      setGomokuEnded();
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
      // Reset rematch UI
      document.getElementById('rematch-area').classList.remove('visible');
      document.getElementById('rematch-count').textContent = '0/0';
      document.getElementById('btn-rematch').disabled = false;
      // Reset ready UI
      document.getElementById('ready-area').style.display = 'none';
      document.getElementById('ready-count').textContent = '0/0';
      document.getElementById('btn-ready').disabled = false;

      // Clear chat
      document.getElementById('chat-messages').innerHTML = '';
    }
  }

  const games = ['omok', 'blackjack', 'tictactoe', 'connect4', 'indian', 'holdem', 'sevenpoker', 'thief', 'onecard', 'mahjong', 'alkkagi'];

  /** record_update ?섏떊 ???꾩쟻 諛곗? 諛??앹뾽??媛깆떊?⑸땲?? */
  function updateRecords(records) {
    if (!records) return;
    const fmt = r => r ? `${r.wins}??${r.losses}??${r.draws}臾? : '0??0??0臾?;
    if (records.total)      document.getElementById('record-total').textContent      = fmt(records.total);
    if (records.omok)       document.getElementById('record-omok').textContent       = fmt(records.omok);
    if (records.blackjack)  document.getElementById('record-blackjack').textContent  = fmt(records.blackjack);
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

  /** 梨꾪똿李??묎린/?쇱튂湲??좉? */
  function toggleChatCollapse() {
    const panel = document.getElementById('chat-panel');
    const btn   = document.getElementById('btn-chat-toggle');
    panel.classList.toggle('collapsed');
    btn.textContent = panel.classList.contains('collapsed') ? '?뮠 ?? : '?뮠 ??;
    btn.title = panel.classList.contains('collapsed') ? '梨꾪똿李??쇱튂湲? : '梨꾪똿李??묎린';
  }

  /** ?꾩쟻 ?앹뾽 ?좉? (?몃? ?대┃ ???ロ옒) */
  function toggleRecordPopup(e) {
    e.stopPropagation();
    document.getElementById('record-popup').classList.toggle('open');
  }
  document.addEventListener('click', () => {
    document.getElementById('record-popup')?.classList.remove('open');
  });

  // ?? Chat System ????????????????????????????????????????????????????????????
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
      el.innerHTML = `<div class="system-msg">?몝 ${escapeHTML(parsed.userId || '')}?섏씠 ?낆옣?덉뒿?덈떎</div>`;
    } else if (type === 'leave') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="system-msg">?슞 ${escapeHTML(parsed.userId || '')}?섏씠 ?댁옣?덉뒿?덈떎</div>`;
    } else if (type === 'game_result') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="result-msg">?룇 ${escapeHTML(parsed.message || '')}</div>`;
    } else if (type === 'game_notice') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="notice-msg">${escapeHTML(parsed.message || '')}</div>`;
    } else if (type === 'system') {
      el.className = 'chat-msg system';
      el.innerHTML = `<div class="system-msg">??${escapeHTML(parsed.message || '')}</div>`;
    }

    messages.appendChild(el);
    messages.scrollTop = messages.scrollHeight;

    // ?묓엺 梨꾪똿李?1以?誘몃━蹂닿린 媛깆떊
    const previewEl = document.getElementById('chat-preview');
    if (previewEl) {
      let text = '';
      if (type === 'chat') {
        text = (parsed.message || '').replace(/^\[.+?\]:\s*/, '');
      } else if (type === 'join') {
        text = `${parsed.userId || ''}?섏씠 ?낆옣?덉뒿?덈떎`;
      } else if (type === 'leave') {
        text = `${parsed.userId || ''}?섏씠 ?댁옣?덉뒿?덈떎`;
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

  // ?? Toast / Modal ??????????????????????????????????????????????????????????
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

  /** ?꾩옱 ?낆옣??諛⑹쓽 ?묐몢?ъ뿉 留욌뒗 猷?紐⑤떖???쒖떆?⑸땲?? */
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
                 : currentRoomId.startsWith('mahjong')   ? 'mahjong'
                 : currentRoomId.startsWith('alkkagi')   ? 'alkkagi'
                 : null;
    if (prefix) showRules(prefix);
  }

  // ?? Rules Modal ????????????????????????????????????????????????????????????
  function showRules(gameId) {
    const g = GAME_CONFIG.find(c => c.id === gameId);
    if (!g) return;
    document.getElementById('rules-title').textContent  = g.ruleTitle;
    document.getElementById('rules-content').innerHTML  = g.ruleHtml;
    document.getElementById('rules-modal').classList.add('show');
  }

  /** ????몃툙?ъ빱 ?꾩슜 議깅낫 ?쒖꽌 紐⑤떖 */
  const POKER_HAND_RANKINGS_HTML = `
    <h3>?뱥 ?ъ빱 議깅낫 ?쒖꽌 (媛?????</h3>
    <ol style="margin:12px 0; padding-left:20px; line-height:2;">
      <li>濡쒗떚??(Royal Flush)</li>
      <li>?ㅽ듃?덉씠???뚮윭??(Straight Flush)</li>
      <li>?ъ뭅??(Four of a Kind)</li>
      <li>??섏슦??(Full House)</li>
      <li>?뚮윭??(Flush)</li>
      <li>?ㅽ듃?덉씠??(Straight)</li>
      <li>?몃━??(Three of a Kind)</li>
      <li>?ы럹??(Two Pair)</li>
      <li>?먰럹??(One Pair)</li>
      <li>?섏씠移대뱶 (High Card)</li>
    </ol>
  `;
  function closeRules() {
    document.getElementById('rules-modal').classList.remove('show');
  }

  // ?? Debug Panel ????????????????????????????????????????????????????????????
  function toggleDebugPanel() {
    document.getElementById('debug-panel').classList.toggle('open');
    document.getElementById('debug-backdrop').classList.toggle('open');
  }

  // ?? Log (debug only) ???????????????????????????????????????????????????????
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
    const LABELS = { sent:'??SENT', recv:'??RECV', error:'??ERROR', info:'??INFO',
                     system:'??SYS', chat:'?뮠 CHAT', 'join-log':'??JOIN', 'leave-log':'??LEAVE',
                     'game-result':'?렡 GAME', 'game-notice':'?뱼 NOTICE', 'board-update':'??BOARD',
                     record:'?룇 RECORD',
                     'bj-state':'?깗 BJ', 'bj-dealer':'?쨼 DEALER' };
    const label = LABELS[type] || type.toUpperCase();
    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span class="log-time">${time}</span> <strong>${label}</strong> ${escapeHTML(formatJSON(data)).slice(0,300)}`;
    logOutput.appendChild(entry);
    logOutput.scrollTop = logOutput.scrollHeight;
  }
  function clearLog() { logOutput.innerHTML = ''; }

  // ?? WebSocket ??????????????????????????????????????????????????????????????
  function connect() {
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    // ?섎뱶肄붾뵫??localhost ??? ?꾩옱 ?묒냽??釉뚮씪?곗????꾨찓???먮뒗 IP)???먮룞?쇰줈 ?곕씪媛寃?蹂寃?    const url = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host + '/ws';
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
    const dot = document.getElementById('conn-dot');
    dot.className = 'connecting';
    document.getElementById('conn-text').textContent = '?곌껐 以?..';
    try {
      ws = new WebSocket(url);
      ws.onopen = () => {
        isIntentionalLeave = false; // ?곌껐 ?깃났 ???뚮옒洹?珥덇린??        setConnectionState(true);
        addLog('info', `?곌껐 ?깃났 ??${url}`);
        // ?곌껐 吏곹썑 JWT瑜??쒕쾭濡??꾩넚?섏뿬 ?몄쬆 (auth guard ?듦낵 議곌굔)
        if (currentToken) {
          sendRaw(JSON.stringify({ action: 'auth', payload: { token: currentToken } }));
        }
        // ?몄쬆 ?댄썑 server?먯꽌 auth_ok瑜?諛쏆쑝硫?pendingJoin 泥섎━??(?꾨옒 case 'auth_ok' 李몄“)
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
              // ?湲?以묒씤 諛??낆옣 ?먮뒗 ?됰꽕???ㅼ젙
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
              const errMsg = parsed.message || '?쒕쾭 ?ㅻ쪟媛 諛쒖깮?덉뒿?덈떎.';
              showToast(errMsg, 'error');
              // 苑?李?諛?媛먯? ???먮룞?쇰줈 濡쒕퉬濡??댁옣 (Auto-kick)
              const fullKeywords = ['?대? 2紐?, '1???꾩슜', '媛??李쇱뒿?덈떎', '?뺤썝 珥덇낵', '?몄썝??媛??, '諛⑹씠 ?댁궛'];
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
                  el.textContent = '?슚 ?뚮젅?댁뼱 ?댁옣. ?쒖엯 ?湲?以?..';
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
              showResultModal(parsed.message || '寃뚯엫??醫낅즺?섏뿀?듬땲??);
              if (parsed.data && parsed.data.board) {
                showGomokuBoard();
                renderBoard({ board: parsed.data.board, turn: '', colors: parsed.data.colors || {}, lastMove: parsed.data.lastMove || [-1,-1] });
                setGomokuEnded();
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
            case 'thief_hover':
              if (parsed.targetId != null && typeof parsed.index === 'number') {
                thiefHoveredTargetId = parsed.targetId;
                thiefHoveredIndex = parsed.index;
                if (lastThiefState) renderThief(lastThiefState);
              }
              break;
            case 'tictactoe_state': case 'connect4_state': case 'indian_state': case 'holdem_state': case 'sevenpoker_state': case 'thief_state': case 'onecard_state': case 'mahjong_state': case 'alkkagi_state': {
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
            case 'dealer_action':
              addLog('bj-dealer', raw);
              renderBlackjackState(parsed.data);
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
        addLog('error', '?곌껐 ?ㅻ쪟. ?쒕쾭媛 ?ㅽ뻾 以묒씤吏 ?뺤씤?섏꽭??');
        document.getElementById('conn-dot').className  = '';
        document.getElementById('conn-text').textContent = '?곌껐 ?ㅻ쪟';
      };
      ws.onclose = (event) => {
        setConnectionState(false);
        addLog('info', `?곌껐 醫낅즺 (code: ${event.code})`);
        ws = null;
        // 濡쒕퉬?먯꽌 ?섎룄???댁옣???꾨땺 ?뚮쭔 3珥????먮룞 ?ъ뿰寃?        if (currentToken && currentMode !== 'room' && !isIntentionalLeave) {
          reconnectTimer = setTimeout(() => {
            addLog('info', '?쒕쾭???ъ뿰寃곗쓣 ?쒕룄?⑸땲??..');
            connect();
          }, 3000);
        }
      };
    } catch (e) {
      addLog('error', `WebSocket ?앹꽦 ?ㅽ뙣: ${e.message}`);
    }
  }

  function disconnect() {
    if (ws) ws.close(1000, 'User disconnected');
  }

  // ?? Join / Leave ???????????????????????????????????????????????????????????

  /** 6?먮━ ?곷Ц+?レ옄 ?쒖닔 肄붾뱶 ?앹꽦 (?? A1B2C3) */
  function genRoomCode() {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // ?쇰룞 臾몄옄(0,O,1,I) ?쒖쇅
    return Array.from({ length: 6 }, () => chars[Math.floor(Math.random() * chars.length)]).join('');
  }

  /** ??諛?留뚮뱾湲? ??6?먮━ 肄붾뱶瑜??앹꽦??利됱떆 ?낆옣 */
  function createRoom(prefix) {
    if (!currentUserId) { showToast('癒쇱? 濡쒓렇?명븯?몄슂.', 'error'); return; }
    const code   = genRoomCode();
    const roomId = prefix + '_' + code;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      pendingJoin = { roomId };
      connect();
    } else {
      sendJoin(roomId);
    }
  }

  /** 肄붾뱶濡??낆옣: ?ъ슜?먭? ?낅젰??6?먮━ 肄붾뱶濡??낆옣 */
  function joinWithCode(prefix, inputId) {
    if (!currentUserId) { showToast('癒쇱? 濡쒓렇?명븯?몄슂.', 'error'); return; }
    const input = document.getElementById(inputId);
    const code  = (input.value || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
    input.value = code;
    if (code.length !== 6) {
      showToast('肄붾뱶???곷Ц+?レ옄 6?먮━瑜??낅젰?섏꽭??', 'error');
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
    // UserID???쒕쾭?먯꽌 auth ?쒖젏???대? ?ㅼ젙?섏뼱 ?덉쓬. payload?먮룄 ?꾨떖 (諛??낆옣 硫붿떆吏 ?쒖떆??
    sendRaw(JSON.stringify({ action: 'join', payload: { roomId, userId: currentUserId } }));
  }

  /** AI 遊?異붽? ?붿껌 (?ㅻぉ/4紐??깊깮??諛⑹뿉?쒕쭔 ?좏슚) */
  function requestAddBot() {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      showToast('?곌껐???딆뼱議뚯뒿?덈떎.', 'error');
      return;
    }
    sendRaw(JSON.stringify({ action: 'add_bot', payload: {} }));
    showToast('AI 遊?異붽? ?붿껌??蹂대깉?듬땲??', 'info');
  }

  /** 諛?肄붾뱶 ?대┰蹂대뱶 蹂듭궗 */
  function copyRoomCode() {
    const code = document.getElementById('room-code-text').textContent.trim();
    if (!code || code === '------') return;
    navigator.clipboard.writeText(code).then(() => {
      showToast('諛?肄붾뱶媛 蹂듭궗?섏뿀?듬땲??', 'success');
    }).catch(() => {
      // fallback for non-https
      const ta = document.createElement('textarea');
      ta.value = code;
      ta.style.cssText = 'position:fixed;opacity:0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
      showToast('諛?肄붾뱶媛 蹂듭궗?섏뿀?듬땲??', 'success');
    });
  }

  function leaveRoom(skipConfirm) {
    isIntentionalLeave = true;
    if (!skipConfirm && !confirm('寃뚯엫 吏꾪뻾 以묒뿉 ?섍?硫??⑤같濡?湲곕줉?????덉뒿?덈떎.\n?뺣쭚 ?섍??쒓쿋?듬땲源?')) {
      isIntentionalLeave = false;
      return;
    }
    sendRaw(JSON.stringify({ action: 'leave', payload: {} }));
    setRoomState('', '');
  }

  // ?? Debug panel manual join ????????????????????????????????????????????????
  function debugJoinRoom() {
    const userId = inputUserId.value.trim();
    const roomId = inputRoomId.value.trim();
    if (!userId || !roomId) return;
    currentUserId = userId;
    sendRaw(JSON.stringify({ action: 'join', payload: { roomId, userId } }));
    toggleDebugPanel();
  }

  // ?? Raw send (used by debug panel) ????????????????????????????????????????
  function sendRaw(text) {
    if (!text || !ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(text);
    addLog('sent', text);
  }

  /** 寃뚯엫 ?≪뀡 ?꾩넚 (愿묓겢 諛⑹?: 0.4珥?荑⑤떎???곸슜) */
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
    try { JSON.parse(text); } catch { addLog('error', '?섎せ??JSON'); return; }
    sendRaw(text);
  }

  msgInput.addEventListener('keydown', (e) => {
    if (e.ctrlKey && e.key === 'Enter') sendMessage();
  });

  // ?? ?ㅻぉ 由щℓ移?????????????????????????????????????????????????????????????

  /** ?ㅻぉ 由щℓ移??붿껌 */
  function sendReady() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
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

  // ?? 寃뚯엫 酉??꾪솚 (DRY) ?????????????????????????????????????????????????????
  const GAME_VIEW_IDS = ['board-placeholder', 'gomoku-container', 'blackjack-container', 'tictactoe-container', 'connect4-container', 'indian-container', 'holdem-container', 'sevenpoker-container', 'thief-container', 'onecard-container', 'mahjong-container', 'alkkagi-container'];
  const PREFIX_TO_CONTAINER = { omok: 'gomoku-container', blackjack: 'blackjack-container', tictactoe: 'tictactoe-container', connect4: 'connect4-container', indian: 'indian-container', holdem: 'holdem-container', sevenpoker: 'sevenpoker-container', thief: 'thief-container', onecard: 'onecard-container', mahjong: 'mahjong-container', alkkagi: 'alkkagi-container' };

  const GAME_STATE_HANDLERS = {
    tictactoe_state:  { logKey: 'ttt-state',       show: showTicTacToeUI,  render: renderTicTacToe },
    connect4_state:   { logKey: 'c4-state',       show: showConnect4UI,   render: renderConnect4 },
    indian_state:     { logKey: 'indian-state',   show: showIndianUI,     render: renderIndian },
    holdem_state:     { logKey: 'holdem-state',   show: showHoldemUI,     render: renderHoldem },
    sevenpoker_state: { logKey: 'sevenpoker-state', show: showSevenPokerUI, render: renderSevenPoker },
    thief_state:      { logKey: 'thief-state',    show: showThiefUI,      render: renderThief },
    onecard_state:    { logKey: 'onecard-state',  show: showOneCardUI,     render: renderOneCard },
    mahjong_state:    { logKey: 'mahjong-state',  show: showMahjongUI,     render: renderMahjong },
    alkkagi_state:    { logKey: 'alkkagi-state',  show: showAlkkagiUI,     render: renderAlkkagi },
  };

  function switchGameView(prefix) {
    const showId = PREFIX_TO_CONTAINER[prefix] || 'board-placeholder';
    GAME_VIEW_IDS.forEach(id => {
      const el = document.getElementById(id);
      if (el) el.style.display = id === showId ? 'flex' : 'none';
    });
    const rematchEl = document.getElementById('rematch-area');
    if (rematchEl) rematchEl.style.display = 'none';
  }

  // ?? Gomoku Board UI ????????????????????????????????????????????????????????
  function showGomokuBoard() {
    switchGameView('omok');
    if (!gomokuBoardReady) { createGomokuBoard(); gomokuBoardReady = true; }
    scaleBoardToFit();
  }

  /** 紐⑤컮?쇱뿉???ㅻぉ 蹂대뱶媛 ?붾㈃ ?덈퉬瑜??섏? ?딅룄濡?transform: scale() ?곸슜 */
  function scaleBoardToFit() {
    const board   = document.getElementById('gomoku-board');
    const scaler  = document.getElementById('gomoku-board-scaler');
    if (!board || !scaler) return;
    const boardW  = 490; // 蹂대뱶 ?ㅼ젣 ?ш린 + ?щ갚 怨좊젮
    const available = window.innerWidth - 20;
    if (available < boardW) {
      const ratio = available / boardW;
      board.style.transform       = `scale(${ratio})`;
      board.style.transformOrigin = 'top center';
      // scaler ?믪씠瑜?異뺤냼??蹂대뱶 ?믪씠??留욎땄 (?덉씠?꾩썐 遺뺢눼 諛⑹?)
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

  function renderBoard(data) {
    if (!gomokuBoardReady) return;
    const board = data.board;
    const prev = gomokuPrevBoard;
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
    gomokuPrevBoard = board.map(row => [...row]);
    const lm = data.lastMove;
    if (lm && lm[0] >= 0) {
      document.querySelector(`.gomoku-cell[data-r="${lm[0]}"][data-c="${lm[1]}"]`)
              ?.querySelector('.gomoku-stone')?.classList.add('last-move');
    }
    if (data.colors) {
      gomokuColorMap = data.colors;
      if (data.colors[currentUserId]) gomokuMyColor = data.colors[currentUserId];
    }
    gomokuTurnUserId = data.turn;
    if (data.turn === currentUserId) {
      cells.forEach(cell => {
        const r = +cell.dataset.r, c = +cell.dataset.c;
        if (data.board[r][c] === 0) cell.classList.add('can-place');
      });
    }

    // 愿?꾩옄 ?먮퀎: colors 留듭뿉 ??ID媛 ?놁쑝硫?愿?꾩옄
    if (data.colors && Object.keys(data.colors).length > 0) {
      const isSpectator = !data.colors[currentUserId];
      if (isSpectator) {
        document.getElementById('rematch-area').classList.remove('visible');
        document.getElementById('gomoku-spectator-msg').style.display = 'block';
      } else {
        document.getElementById('gomoku-spectator-msg').style.display = 'none';
        // 由щℓ移?踰꾪듉? game_result ?대깽?멸? 蹂꾨룄濡?泥섎━
      }
    }
    updateStatusBar(data.turn);
    updateColorInfo(data.colors);
  }

  function updateStatusBar(turnUserId) {
    const stoneEl = document.getElementById('status-stone-icon');
    const userEl  = document.getElementById('status-turn-user');
    if (!stoneEl || !userEl) return;
    if (turnUserId) {
      const col = gomokuColorMap[turnUserId];
      stoneEl.textContent = col === 1 ? '?? : col === 2 ? '?? : '??;
      const isMine = turnUserId === currentUserId;
      const suffix = isMine ? ' (??' : '';
      userEl.innerHTML = `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(turnUserId)}')" title="?꾩쟻 蹂닿린">${escapeHTML(turnUserId)}</span>${suffix}`;
      userEl.style.color = isMine ? '#3fb950' : 'var(--text-primary)';
    }
  }

  /** currentRoomId ?묐몢?ъ뿉 留욌뒗 ??대㉧ ?붿냼瑜?李얠븘 ?낅뜲?댄듃 (5珥??댄븯 ??urgent ?대옒?? */
  const PREFIX_TO_TIMER = { omok: ['status-seconds', 'status-timer-block'], tictactoe: ['ttt-seconds', 'ttt-timer-block'], connect4: ['c4-seconds', 'c4-timer-block'], indian: ['indian-seconds', 'indian-timer-block'], holdem: ['holdem-seconds', 'holdem-timer-block'], sevenpoker: ['sevenpoker-seconds', 'sevenpoker-timer-block'], thief: ['thief-seconds', 'thief-timer-block'], onecard: ['onecard-seconds', 'onecard-timer-block'], mahjong: ['mahjong-seconds', 'mahjong-timer-block'] };

  function updateGameTimer(turnUser, remaining) {
    const prefix = currentRoomId.startsWith('omok') ? 'omok'
                 : currentRoomId.startsWith('tictactoe') ? 'tictactoe'
                 : currentRoomId.startsWith('connect4') ? 'connect4'
                 : currentRoomId.startsWith('indian') ? 'indian'
                 : currentRoomId.startsWith('holdem') ? 'holdem'
                 : currentRoomId.startsWith('sevenpoker') ? 'sevenpoker'
                 : currentRoomId.startsWith('thief') ? 'thief'
                 : currentRoomId.startsWith('onecard') ? 'onecard'
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

  function updateColorInfo(colors) {
    const el = document.getElementById('gomoku-color-info');
    if (!colors || !Object.keys(colors).length) return;
    el.innerHTML = Object.entries(colors)
      .map(([uid, col]) => `${col === 1 ? '?? : '??} <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="?꾩쟻 蹂닿린">${escapeHTML(uid)}</span>`)
      .join('  vs  ');
  }

  function setGomokuEnded() {
    gomokuTurnUserId = '';
    gomokuEnded      = true;
    const userEl = document.getElementById('status-turn-user');
    if (userEl) userEl.textContent = '寃뚯엫 醫낅즺';
    const stoneEl = document.getElementById('status-stone-icon');
    if (stoneEl) stoneEl.textContent = '?뢾';
    const secsEl = document.getElementById('status-seconds');
    if (secsEl) secsEl.textContent = '--';
    document.getElementById('status-timer-block')?.classList.remove('urgent');
    document.querySelectorAll('.gomoku-cell').forEach(c => c.classList.remove('can-place'));
  }

  function onGomokuCellClick(r, c) {
    if (!currentRoomId || !ws || ws.readyState !== WebSocket.OPEN) return;
    if (gomokuTurnUserId !== currentUserId) return;
    sendGameAction({ cmd: 'place', x: r, y: c });
  }

  // ?? Blackjack UI ???????????????????????????????????????????????????????????
  function showBlackjackUI() {
    switchGameView('blackjack');
  }

  function renderBlackjackState(data) {
    if (!data) return;
    renderBJHand('bj-dealer-hand', 'bj-dealer-score', data.dealerHand);
    renderBJHand('bj-player-hand', 'bj-player-score', data.playerHand);
    const msgEl = document.getElementById('bj-message');
    const overlayEl = document.getElementById('bj-result-overlay');
    const boxEl = document.getElementById('bj-result-box');
    const msgTextEl = document.getElementById('bj-result-msg');
    if (data.message) msgEl.textContent = data.message;

    const showResult = (data.phase === 'settlement' || data.state === 'game_over') && data.message;
    if (showResult) {
      msgTextEl.textContent = data.message;
      boxEl.className = 'unified-result-box';
      if (/?밸━|Win|?닿꼈|釉붾옓??i.test(data.message)) boxEl.classList.add('win');
      else if (/?⑤같|Lose|議?踰꾩뒪??Bust/i.test(data.message)) boxEl.classList.add('lose');
      else boxEl.classList.add('push');

      overlayEl.style.display = 'flex';

      if (window.bjResultTimer) clearTimeout(window.bjResultTimer);
      window.bjResultTimer = setTimeout(() => {
        overlayEl.style.display = 'none';
      }, 3500);
    } else {
      overlayEl.style.display = 'none';
    }

    const startBtns    = document.getElementById('bj-start-buttons');
    const gameBtns     = document.getElementById('bj-game-buttons');
    const spectatorMsg = document.getElementById('bj-spectator-msg');

    // 愿?꾩옄 ?먮퀎: mainPlayerId媛 ?덇퀬 ?섏? ?ㅻⅤ硫?愿?꾩옄
    const isSpectator = data.mainPlayerId && data.mainPlayerId !== currentUserId;
    if (isSpectator) {
      startBtns.style.display    = 'none';
      gameBtns.style.display     = 'none';
      spectatorMsg.style.display = 'block';
    } else {
      spectatorMsg.style.display = 'none';
      if (data.phase === 'betting' || data.phase === 'settlement') {
        startBtns.style.display = 'flex'; gameBtns.style.display = 'none';
      } else if (data.phase === 'player_turn') {
        startBtns.style.display = 'none'; gameBtns.style.display = 'flex';
      } else {
        startBtns.style.display = 'none'; gameBtns.style.display = 'none';
      }
    }
  }

  function renderBJHand(handElId, scoreElId, handInfo) {
    const handEl  = document.getElementById(handElId);
    const scoreEl = document.getElementById(scoreElId);
    if (!handInfo || !handInfo.cards || handInfo.cards.length === 0) {
      handEl.innerHTML = ''; scoreEl.textContent = ''; scoreEl.className = 'bj-score'; return;
    }
    handEl.innerHTML = handInfo.cards.map(card => {
      if (card.hidden) return `<div class="bj-card hidden"></div>`;
      const isRed = card.suit === '?? || card.suit === '??;
      return `<div class="bj-card ${isRed ? 'red' : 'black'}">
        <span class="bj-card-top">${card.value}</span>
        <span class="bj-card-center">${card.suit}</span>
        <span class="bj-card-bot">${card.value}</span>
      </div>`;
    }).join('');
    const score = handInfo.score;
    scoreEl.textContent = score > 0 ? `${score}` : '';
    scoreEl.className   = score > 21 ? 'bj-score bust' : 'bj-score';
  }

  function bjStart() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !currentRoomId) return;
    sendGameAction({ cmd: 'start' });
  }
  function bjHit() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'hit' });
  }
  function bjStand() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'stand' });
  }

  // ?? Tictactoe UI ???????????????????????????????????????????????????????????

  let tttMyColor = 0;    // 1=O, 2=X (0=誘몄젙)
  let tttTurnId  = '';   // ?꾩옱 李⑤????좎? ID

  function showTicTacToeUI() {
    switchGameView('tictactoe');
  }

  function renderTicTacToe(data) {
    if (!data) return;
    tttTurnId = data.turn || '';

    // ?????뺤씤
    if (data.colors && data.colors[currentUserId]) {
      tttMyColor = data.colors[currentUserId];
    }

    // ?곹깭 諛??띿뒪??    const statusEl = document.getElementById('ttt-status');
    if (statusEl) {
      if (tttTurnId === currentUserId) {
        const sym = tttMyColor === 1 ? '狩? : '??;
        statusEl.textContent = `${sym} ??李⑤??낅땲??`;
        statusEl.style.color = 'var(--accent)';
      } else if (tttTurnId) {
        statusEl.innerHTML = `??<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(tttTurnId)}')" title="?꾩쟻 蹂닿린">${escapeHTML(tttTurnId)}</span>??李⑤?...`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // ?됱긽 ?뺣낫
    const infoEl = document.getElementById('ttt-color-info');
    if (infoEl && data.colors) {
      infoEl.innerHTML = Object.entries(data.colors)
        .map(([uid, col]) => `${col === 1 ? '狩?O' : '??X'}: <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="?꾩쟻 蹂닿린">${escapeHTML(uid)}</span>`)
        .join('  |  ');
    }

    // 3횞3 蹂대뱶 ?뚮뜑留?(洹몃━???좎? + diffing)
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
    cells.forEach(cell => {
      const r = +cell.dataset.r, c = +cell.dataset.c;
      const val = board[r][c];
      const wasEmpty = prev && prev[r][c] === 0;
      const isNew = wasEmpty && (val === 1 || val === 2);
      cell.classList.remove('ttt-o', 'ttt-x', 'ttt-can-place');
      cell.innerHTML = '';
      cell.onclick = null;
      if (val === 1) {
        cell.classList.add('ttt-o');
        const inner = document.createElement('span');
        inner.className = 'ttt-cell-inner' + (isNew ? ' animate-pop' : '');
        inner.textContent = '狩?;
        cell.appendChild(inner);
      } else if (val === 2) {
        cell.classList.add('ttt-x');
        const inner = document.createElement('span');
        inner.className = 'ttt-cell-inner' + (isNew ? ' animate-pop' : '');
        inner.textContent = '??;
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

  // ?? Connect 4 UI ???????????????????????????????????????????????????????????

  let c4MyColor = 0;  // 1=鍮④컯, 2=?몃옉 (0=誘몄젙)
  let c4TurnId  = ''; // ?꾩옱 李⑤????좎? ID

  function showConnect4UI() {
    switchGameView('connect4');
  }

  function renderConnect4(data) {
    if (!data) return;
    c4TurnId = data.turn || '';

    // ?????뺤씤
    if (data.colors && data.colors[currentUserId]) {
      c4MyColor = data.colors[currentUserId];
    }

    const isMyTurn = c4TurnId === currentUserId;

    // ?곹깭 諛?    const statusEl = document.getElementById('c4-status');
    if (statusEl) {
      if (isMyTurn) {
        const sym = c4MyColor === 1 ? '?뵶' : '?윞';
        statusEl.textContent = `${sym} ??李⑤??낅땲?? ?댁쓣 ?좏깮?섏꽭??`;
        statusEl.style.color = 'var(--accent)';
      } else if (c4TurnId) {
        statusEl.innerHTML = `??<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(c4TurnId)}')" title="?꾩쟻 蹂닿린">${escapeHTML(c4TurnId)}</span>??李⑤?...`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // ?됱긽 ?뺣낫
    const infoEl = document.getElementById('c4-color-info');
    if (infoEl && data.colors) {
      infoEl.innerHTML = Object.entries(data.colors)
        .map(([uid, col]) => `${col === 1 ? '?뵶 鍮④컯' : '?윞 ?몃옉'}: <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(uid)}')" title="?꾩쟻 蹂닿린">${escapeHTML(uid)}</span>`)
        .join('  |  ');
    }

    // ???좏깮 踰꾪듉 ?뚮뜑留?    const colBtnsEl = document.getElementById('c4-col-btns');
    if (colBtnsEl) {
      colBtnsEl.innerHTML = '';
      for (let c = 0; c < 7; c++) {
        const btn = document.createElement('button');
        btn.className = 'c4-col-btn';
        btn.textContent = '??;
        btn.disabled = !isMyTurn;
        if (isMyTurn) {
          btn.addEventListener('click', () => onC4ColClick(c));
        }
        colBtnsEl.appendChild(btn);
      }
    }

    // 6횞7 蹂대뱶 ?뚮뜑留?(洹몃━???좎? + diffing)
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

  // ?? Indian Poker UI ????????????????????????????????????????????????????????

  function showIndianUI() {
    lastIndianOppCard = '';
    lastIndianMyCard  = '';
    switchGameView('indian');
  }

  /** ?섑듃 諛?HTML ?앹꽦 (湲곕낯 10移? 10媛?珥덇낵 ??理쒕? 20媛쒓퉴吏 ?꾩씠肄??뺤옣) */
  function renderHeartsBar(count) {
    const MAX_ICONS = 20; // ?꾩씠肄섏쑝濡??쒖떆??理쒕? 媛쒖닔
    const displayMax = Math.max(10, count); // 湲곕낯 10移??좎?, ?섏쑝硫?洹몃쭔???щ’ ?뺤옣

    let html = '';
    const iconCount = Math.min(count, MAX_ICONS);
    const totalSlots = Math.min(displayMax, MAX_ICONS);

    for (let i = 0; i < totalSlots; i++) {
      html += `<span class="indian-heart${i >= iconCount ? ' lost' : ''}">?ㅿ툘</span>`;
    }
    if (count > MAX_ICONS) {
      html += `<span class="indian-hearts-count">+${count - MAX_ICONS}</span>`;
    }
    return html;
  }

  /** 移대뱶 HTML ?앹꽦 */
  function renderIndianCard(card) {
    if (!card || card.hidden) {
      // ?룸㈃
      return `<div class="indian-card-back">?깗</div>`;
    }
    const isRed = card.suit === '?? || card.suit === '??;
    return `
      <div class="indian-card-face ${isRed ? 'red-suit' : 'black-suit'}">
        <div class="indian-card-val-top">${card.value}</div>
        <div class="indian-card-suit-center">${card.suit}</div>
        <div class="indian-card-val-bot">${card.value}</div>
      </div>`;
  }

  function renderIndian(data) {
    if (!data) return;

    const isMyTurn = data.turn === currentUserId;
    const isSpectator = data.myName !== currentUserId && data.opponentName !== currentUserId;

    // ?쇱슫??諛?    const roundEl = document.getElementById('indian-round-bar');
    if (roundEl) roundEl.textContent = `?쇱슫??${data.round}`;

    // ?곹깭 諛?    const statusEl = document.getElementById('indian-status');
    if (statusEl) {
      if (data.phase === 'waiting') {
        statusEl.textContent = '?곷?諛⑹쓣 湲곕떎由щ뒗 以?..';
      } else if (isMyTurn && !isSpectator) {
        statusEl.textContent = data.phase === 'first_action'
          ? '?렞 ??李⑤? ???밸? ?먮뒗 ?ш린瑜??좏깮?섏꽭??'
          : '?렞 ??李⑤? ??肄??밸?) ?먮뒗 ?ш린瑜??좏깮?섏꽭??';
        statusEl.style.color = 'var(--accent)';
      } else {
        statusEl.innerHTML = `??<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(data.turn)}')" title="?꾩쟻 蹂닿린">${escapeHTML(data.turn)}</span>???좏깮??湲곕떎由щ뒗 以?..`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // ?곷?諛??뺣낫
    const oppName = data.opponentName || '??;
    document.getElementById('indian-opp-name').innerHTML = oppName !== '??
      ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(oppName)}')" title="?꾩쟻 蹂닿린">${escapeHTML(oppName)}</span>`
      : '??;
    document.getElementById('indian-opp-hearts').innerHTML = renderHeartsBar(data.opponentHearts);
    const oppCardHtml = renderIndianCard(data.opponentCard);
    if (lastIndianOppCard !== oppCardHtml) {
      document.getElementById('indian-opp-card-wrap').innerHTML = oppCardHtml;
      lastIndianOppCard = oppCardHtml;
    }

    // ???뺣낫
    const myName = data.myName || '??;
    document.getElementById('indian-my-name').innerHTML = myName !== '??
      ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(myName)}')" title="?꾩쟻 蹂닿린">${escapeHTML(myName)}</span>`
      : '??;
    document.getElementById('indian-my-hearts').innerHTML = renderHeartsBar(data.myHearts);
    const myCardHtml = renderIndianCard(data.myCard);
    if (lastIndianMyCard !== myCardHtml) {
      document.getElementById('indian-my-card-wrap').innerHTML = myCardHtml;
      lastIndianMyCard = myCardHtml;
    }

    // ?꾩옱 ???좎? ?섑띁??active-turn 媛뺤“
    const oppArea = document.querySelector('.indian-player-area.opponent-area');
    const myArea = document.querySelector('.indian-player-area.my-area');
    if (oppArea) oppArea.classList.toggle('active-turn', data.turn === data.opponentName);
    if (myArea) myArea.classList.toggle('active-turn', data.turn === data.myName);

    // ?≪뀡 踰꾪듉 ?쒖꽦??(???댁씠怨?愿?꾩옄媛 ?꾨땺 ??
    const canAct = isMyTurn && !isSpectator && data.phase !== 'waiting';
    document.getElementById('btn-indian-showdown').disabled = !canAct;
    document.getElementById('btn-indian-giveup').disabled   = !canAct;
  }

  function indianShowdown() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'showdown' });
  }

  function indianGiveUp() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'give_up' });
  }

  // ?? Thief (?꾨몣?↔린) UI ????????????????????????????????????????????????????

  let thiefHoveredIndex = -1;
  let thiefHoveredTargetId = '';
  let lastThiefState = null;

  function showThiefUI() {
    switchGameView('thief');
    thiefHoveredIndex = -1;
    thiefHoveredTargetId = '';
  }

  function renderThiefCard(card, hoverFinger, isDiscarding) {
    if (!card) return '';
    const isRed = card.suit === '?? || card.suit === '??;
    const discardClass = isDiscarding ? ' discarding-pair' : '';
    const suit = card.suit || '?깗';
    const val = card.value || '?';
    const fingerClass = hoverFinger ? ' hover-finger' : '';
    return `<div class="thief-card ${isRed ? 'red-suit' : 'black-suit'}${fingerClass}${discardClass}"><span>${val}</span><span>${suit}</span></div>`;
  }

  const TABLE_SEAT_ORDER = ['seat-top', 'seat-left', 'seat-right'];
  let lastThiefHandJson = '';

  function renderThief(data) {
    if (!data) return;
    lastThiefState = data;
    const isMyTurn = data.turn === currentUserId;
    document.getElementById('thief-status').textContent = isMyTurn
      ? '?렞 ??李⑤? ???곷?諛?移대뱶瑜??대┃?섏뿬 戮묒쑝?몄슂!'
      : `??${escapeHTML(data.turn || '??)}??李⑤?`;
    document.getElementById('thief-escaped').textContent = data.escaped && data.escaped.length
      ? `?덉텧: ${data.escaped.join(', ')}`
      : '';

    const players = data.players || [];
    const opponents = players.filter(p => p.userId !== currentUserId);
    const playersEl = document.getElementById('thief-players');
    if (playersEl) {
      playersEl.innerHTML = opponents.map((p, idx) => {
        const seatClass = TABLE_SEAT_ORDER[idx] || 'seat-top';
        const isTarget = data.targetUserId === p.userId;
        const cardCount = p.cardCount || 0;
        let targetCardsHtml = '';
        if (isTarget && cardCount > 0) {
          targetCardsHtml = '<div class="table-seat-target-cards">' + Array.from({ length: cardCount }, (_, i) => {
            const hovered = thiefHoveredTargetId === p.userId && thiefHoveredIndex === i;
            return `<div class="thief-target-card${hovered ? ' hovered' : ''}" data-target-id="${escapeHTML(p.userId)}" data-index="${i}">?깗</div>`;
          }).join('') + '</div>';
        }
        const isTheirTurn = data.turn === p.userId;
        return `<div class="table-seat ${seatClass} ${isTheirTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <span class="table-seat-count">?깗 ${cardCount}??/span>
          ${targetCardsHtml}
        </div>`;
      }).join('');
      playersEl.querySelectorAll('.thief-target-card').forEach(el => {
        el.onclick = () => {
          const targetId = el.dataset.targetId;
          const index = parseInt(el.dataset.index, 10);
          thiefOnTargetCardClick(targetId, index, el);
        };
      });
    }

    const handEl = document.getElementById('thief-hand');
    if (handEl && data.hand) {
      const discardingSet = new Set((data.discardingPairs || []));
      const handJson = JSON.stringify(data.hand);
      if (handJson !== lastThiefHandJson) {
        lastThiefHandJson = handJson;
        const hoverOnMyCard = thiefHoveredTargetId === currentUserId;
        handEl.innerHTML = data.hand.map((c, i) => renderThiefCard(c, hoverOnMyCard && thiefHoveredIndex === i, discardingSet.has(i))).join('');
      } else {
        const hoverOnMyCard = thiefHoveredTargetId === currentUserId;
        handEl.querySelectorAll('.thief-card').forEach((el, i) => {
          el.classList.toggle('hover-finger', hoverOnMyCard && thiefHoveredIndex === i);
          el.classList.toggle('discarding-pair', discardingSet.has(i));
        });
      }
    }

    setTimeout(() => {
      document.querySelectorAll('.table-seat').forEach(el => el.classList.remove('target-seat'));
      if (data.targetUserId) {
        document.querySelectorAll('.table-seat').forEach(el => {
          if (el.getAttribute('data-user-id') === data.targetUserId) {
            el.classList.add('target-seat');
          }
        });
      }
    }, 50);
  }

  function thiefOnTargetCardClick(targetId, index, el) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (thiefHoveredTargetId === targetId && thiefHoveredIndex === index) {
      if (window.isDrawing) return;
      window.isDrawing = true;
      setTimeout(() => { window.isDrawing = false; }, 500);
      sendGameAction({ cmd: 'draw', targetId: targetId, index: index });
      thiefHoveredIndex = -1;
      thiefHoveredTargetId = '';
    } else {
      sendGameAction({ cmd: 'hover', targetId: targetId, index: index });
      thiefHoveredIndex = index;
      thiefHoveredTargetId = targetId;
      document.querySelectorAll('#thief-players .thief-target-card').forEach(c => c.classList.remove('hovered'));
      if (el) el.classList.add('hovered');
    }
  }

  function thiefDraw() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'draw' });
  }

  // ?? OneCard (?먯뭅?? UI ????????????????????????????????????????????????????

  function showOneCardUI() {
    switchGameView('onecard');
  }

  function renderOneCardCard(card, playable) {
    if (!card) return '';
    const isRed = card.suit === '?? || card.suit === '??;
    const suit = card.suit || '?깗';
    let val = card.value || '?';
    if (val === 'B_JOKER') val = 'B';
    if (val === 'C_JOKER') val = 'C';
    return `<div class="onecard-card ${isRed ? 'red-suit' : 'black-suit'} ${playable ? 'playable' : ''}" data-index="${card._index ?? ''}"><span>${val}</span><span>${suit}</span></div>`;
  }

  function onecardCanDefend(attackCard, card) {
    if (!attackCard || !card) return false;
    const av = attackCard.value || '';
    const cv = card.value || '';
    if (av === 'A') return cv === 'A' || cv === 'B_JOKER' || cv === 'C_JOKER';
    if (av === 'B_JOKER') return cv === 'C_JOKER';
    if (av === 'C_JOKER') return false;
    return false;
  }
  function onecardIsPlayable(data, card) {
    const top = data.topCard || {};
    const suit = data.targetSuit || top.suit || '';
    const attackPenalty = data.attackPenalty || 0;
    if (attackPenalty > 0) return onecardCanDefend(top, card);
    const cv = card.value || '';
    if (cv === 'B_JOKER' || cv === 'C_JOKER') return true;
    return (card.suit === suit || card.value === top.value);
  }

  let onecardPendingPlayIndex = -1;

  let spChoiceDiscard = -1;
  let spChoiceOpen = -1;
  let spChoiceMyCards = [];

  function renderOneCard(data) {
    if (!data) return;
    const isMyTurn = data.turn === currentUserId;
    const top = data.topCard || {};
    const attackPenalty = data.attackPenalty || 0;
    const oneCardVuln = data.oneCardVulnerable || '';

    document.getElementById('onecard-status').textContent = isMyTurn
      ? (attackPenalty > 0 ? '?썳截?諛⑹뼱 移대뱶瑜??닿굅???쒕줈?고븯?몄슂!' : '?렞 ??李⑤? ??移대뱶瑜??닿굅???쒕줈?고븯?몄슂!')
      : `??${escapeHTML(data.turn || '??)}??李⑤?`;
    document.getElementById('onecard-direction').textContent = (data.direction === -1 ? '??諛섏떆怨? : '???쒓퀎') + ' 諛⑺뼢';

    const attackBanner = document.getElementById('onecard-attack-banner');
    const attackCount = document.getElementById('onecard-attack-count');
    if (attackBanner && attackCount) {
      attackCount.textContent = attackPenalty;
      attackBanner.classList.toggle('show', attackPenalty > 0);
    }

    const btnCall = document.getElementById('btn-onecard-call');
    if (btnCall) {
      btnCall.disabled = !oneCardVuln || !ws || ws.readyState !== WebSocket.OPEN;
    }

    const playersEl = document.getElementById('onecard-players');
    if (playersEl && data.players) {
      const opponents = (data.players || []).filter(p => p.userId !== currentUserId);
      playersEl.innerHTML = opponents.map((p, idx) => {
        const seatClass = TABLE_SEAT_ORDER[idx] || 'seat-top';
        return `<div class="table-seat onecard-player-box ${seatClass} ${p.isTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <span class="table-seat-count">?깗 ${p.cardCount || 0}??/span>
        </div>`;
      }).join('');
    }

    const topEl = document.getElementById('onecard-top-card');
    if (topEl) topEl.innerHTML = top.suit ? renderOneCardCard(top, false).replace(' data-index=""', '') : '';
    const deckEl = document.getElementById('onecard-deck');
    if (deckEl) {
      const total = (data.deckCount || 0) + (data.discardCount || 0);
      deckEl.textContent = total > 0 ? `?깗 ${data.deckCount || 0}` : '';
      deckEl.style.cursor = isMyTurn && total > 0 ? 'pointer' : 'default';
      deckEl.onclick = (total > 0 && isMyTurn) ? onecardDraw : null;
    }
    const handEl = document.getElementById('onecard-hand');
    if (handEl && data.hand) {
      const canPlay = isMyTurn && top.suit;
      handEl.innerHTML = data.hand.map((c, i) => {
        const playable = canPlay && onecardIsPlayable(data, c);
        const cardWithIdx = { ...c, _index: i };
        return renderOneCardCard(cardWithIdx, playable);
      }).join('');
      handEl.querySelectorAll('.onecard-card.playable').forEach(el => {
        el.style.cursor = 'pointer';
        el.onclick = () => {
          const idx = parseInt(el.dataset.index, 10);
          if (isNaN(idx)) return;
          const card = data.hand[idx];
          if (card && card.value === '7') {
            onecardPendingPlayIndex = idx;
            document.getElementById('onecard-suit-modal').classList.add('show');
          } else {
            onecardPlay(idx);
          }
        };
      });
    }
  }

  function closeOneCardSuitModal() {
    document.getElementById('onecard-suit-modal').classList.remove('show');
    onecardPendingPlayIndex = -1;
  }
  function onecardPickSuit(suit) {
    if (onecardPendingPlayIndex < 0) return;
    onecardPlay(onecardPendingPlayIndex, suit);
    closeOneCardSuitModal();
  }

  function onecardDraw() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'draw' });
  }

  function onecardPlay(index, targetSuit) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const payload = { cmd: 'play', index };
    if (targetSuit) payload.targetSuit = targetSuit;
    sendGameAction(payload);
  }

  function onecardCallOneCard() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'call_onecard' });
  }

  // ?? Mahjong (留덉옉) UI ???????????????????????????????????????????????????????

  function showMahjongUI() {
    switchGameView('mahjong');
  }

  function getMahjongTileHTML(type, value, isHidden = false) {
    if (isHidden) return '<div class="mahjong-tile hidden">??/div>';
    const dict = {
      'man': ['','??,'??,'??,'??,'??,'??,'??,'??,'??],
      'pin': ['','??,'??,'??,'??,'??,'??,'??,'??,'??],
      'sou': ['','??,'??,'??,'??,'??,'??,'??,'??,'??],
      'honor': ['','?','??,'??,'??,'??,'??,'??]
    };
    const char = dict[type] ? dict[type][value] : '??;
    return `<div class="mahjong-tile">${char}</div>`;
  }

  function mahjongTileChar(tile) {
    if (!tile || !tile.type) return '??;
    const man = ['', '??,'??,'??,'??,'??,'??,'??,'??,'??];
    const pin = ['', '??,'??,'??,'??,'??,'??,'??,'??,'??];
    const sou = ['', '??,'??,'??,'??,'??,'??,'??,'??,'??];
    const honor = ['', '?','??,'??,'??,'??,'??,'??];
    const v = tile.value || 0;
    if (tile.type === 'man' && v >= 1 && v <= 9) return man[v];
    if (tile.type === 'pin' && v >= 1 && v <= 9) return pin[v];
    if (tile.type === 'sou' && v >= 1 && v <= 9) return sou[v];
    if (tile.type === 'honor' && v >= 1 && v <= 7) return honor[v];
    return '??;
  }

  function renderMahjong(data) {
    if (!data) return;
    const isMyTurn = data.currentTurn === currentUserId;
    const myHand = data.myHand || [];
    const canDiscard = isMyTurn && myHand.length === 14;

    document.getElementById('mahjong-status').textContent = isMyTurn
      ? '?렞 ??李⑤? ???⑤? ?대┃??踰꾨━?몄슂!'
      : `??${escapeHTML(data.currentTurn || '??)}??李⑤?`;
    document.getElementById('mahjong-wall-info').textContent = `???⑥? ?? ${data.wallCount ?? 0}??;

    const playersEl = document.getElementById('mahjong-players');
    if (playersEl && data.players) {
      const myIdx = data.players.findIndex(p => p && p.userId === currentUserId);
      const opponentIndices = myIdx >= 0 ? [(myIdx + 2) % 4, (myIdx + 3) % 4, (myIdx + 1) % 4] : [0, 1, 2];
      const opponents = opponentIndices.map(i => data.players[i]).filter(p => p && p.userId);
      playersEl.innerHTML = opponents.map((p, idx) => {
        const seatClass = TABLE_SEAT_ORDER[idx] || 'seat-top';
        const discardsHtml = (p.discards || []).map(t => getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)).join('');
        const handHtml = Array(p.handCount || 0).fill(0).map(() => getMahjongTileHTML('', 0, true)).join('');
        return `<div class="table-seat mahjong-player-box ${seatClass} ${p.isTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <div class="mahjong-discards mahjong-discards-row">${discardsHtml}</div>
          <div class="mahjong-hand opponent-hand">${handHtml}</div>
        </div>`;
      }).join('');
    }

    const discardsMeEl = document.getElementById('mahjong-discards-me');
    const mePlayer = data.players?.find(p => p.userId === currentUserId);
    if (discardsMeEl && mePlayer) {
      discardsMeEl.innerHTML = (mePlayer.discards || []).map(t => getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)).join('');
    }

    const handEl = document.getElementById('mahjong-hand');
    if (handEl && myHand) {
      handEl.innerHTML = myHand.map((t, i) => {
        const discardable = canDiscard ? ' discardable' : '';
        const type = t.type || t.Type;
        const value = t.value ?? t.Value ?? 0;
        return `<div class="mahjong-tile${discardable}" data-index="${i}">${getMahjongTileHTML(type, value, false).replace(/^<div[^>]*>|<\/div>$/g, '')}</div>`;
      }).join('');
      if (canDiscard) {
        handEl.querySelectorAll('.mahjong-tile.discardable').forEach(el => {
          el.style.cursor = 'pointer';
          el.onclick = () => {
            const idx = parseInt(el.dataset.index, 10);
            if (!isNaN(idx)) mahjongDiscard(idx);
          };
        });
      }
    }
  }

  function mahjongDiscard(index) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'discard', index });
  }

  // ?? Alkkagi UI (Matter.js 臾쇰━ 蹂대뱶) ????????????????????????????????????????

  let alkkagiEngine = null;
  let alkkagiRender = null;
  let alkkagiRunner = null;
  let alkkagiWorld = null;
  let alkkagiBodies = {};  // id -> Matter.Body
  let alkkagiInitialized = false;

  function showAlkkagiUI() {
    switchGameView('alkkagi');
  }

  function renderAlkkagi(data) {
    if (!data) return;
    const statusEl = document.getElementById('alkkagi-status');
    if (statusEl) {
      const turn = data.currentTurn || '';
      statusEl.textContent = turn === currentUserId
        ? '?렞 ??李⑤? ???뚯쓣 ?뺢꺼 ?곷? ?뚯쓣 諛?대궡?몄슂!'
        : turn ? `??${escapeHTML(turn)}??李⑤?` : '?뚭퉴湲????곷?諛⑹쓣 湲곕떎由щ뒗 以?..';
    }

    const wrap = document.getElementById('alkkagi-board-wrap');
    if (!wrap || typeof Matter === 'undefined') return;

    if (!alkkagiInitialized) {
      alkkagiInitialized = true;
      initAlkkagiPhysics(wrap, data);
    } else {
      syncAlkkagiStones(data.stones || []);
    }
  }

  function initAlkkagiPhysics(container, data) {
    const M = Matter;
    const W = 420, H = 420;
    const wallThick = 15;

    const engine = M.Engine.create();
    engine.gravity.x = 0;
    engine.gravity.y = 0;

    const world = engine.world;

    // 4硫?踰?(?뺤쟻 諛붾뵒)
    const walls = [
      M.Bodies.rectangle(W/2, -wallThick/2, W + wallThick*2, wallThick, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(W/2, H + wallThick/2, W + wallThick*2, wallThick, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(-wallThick/2, H/2, wallThick, H + wallThick*2, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(W + wallThick/2, H/2, wallThick, H + wallThick*2, { isStatic: true, render: { fillStyle: '#7a5010' } }),
    ];
    M.World.add(world, walls);

    const stoneRadius = 18;
    const stoneOpts = { friction: 0.01, frictionAir: 0.008, restitution: 0.6, density: 0.001 };

    const stones = data.stones || [];
    stones.forEach(s => {
      const fill = s.color === 1 ? '#111' : '#f5f5f5';
      const stroke = s.color === 1 ? '#333' : '#ccc';
      const body = M.Bodies.circle(s.x || 100, s.y || 100, stoneRadius, {
        ...stoneOpts,
        render: { fillStyle: fill, strokeStyle: stroke, lineWidth: 2 },
      });
      body.alkkagiId = s.id;
      body.alkkagiColor = s.color;
      alkkagiBodies[s.id] = body;
      M.World.add(world, body);
    });

    const render = M.Render.create({
      element: container,
      engine: engine,
      options: {
        width: W + wallThick*2,
        height: H + wallThick*2,
        wireframes: false,
        background: '#c8a45a',
      },
    });
    M.Render.run(render);

    const mouseConstraint = M.MouseConstraint.create(engine, {
      element: render.canvas,
      constraint: { stiffness: 0.2, render: { visible: false } },
    });
    M.World.add(world, mouseConstraint);

    const runner = M.Runner.create();
    M.Runner.run(runner, engine);

    alkkagiEngine = engine;
    alkkagiRender = render;
    alkkagiRunner = runner;
    alkkagiWorld = world;
  }

  function syncAlkkagiStones(stones) {
    if (!alkkagiWorld || !alkkagiBodies) return;
    stones.forEach(s => {
      const body = alkkagiBodies[s.id];
      if (body) {
        Matter.Body.setPosition(body, { x: s.x, y: s.y });
        Matter.Body.setVelocity(body, { x: s.velX || 0, y: s.velY || 0 });
      }
    });
  }

  // ?? Texas Holdem UI ???????????????????????????????????????????????????????

  function showHoldemUI() {
    switchGameView('holdem');
  }

  function renderHoldemCard(card) {
    if (!card || card.hidden) {
      return `<div class="holdem-card hidden">?깗</div>`;
    }
    const isRed = card.suit === '?? || card.suit === '??;
    return `<div class="holdem-card ${isRed ? 'red-suit' : 'black-suit'}">
      <span class="holdem-card-top">${card.value}</span>
      <span class="holdem-card-suit">${card.suit}</span>
      <span class="holdem-card-bot">${card.value}</span>
    </div>`;
  }

  function renderHoldem(data) {
    if (!data) return;

    const isMyTurn = data.currentTurn === currentUserId;
    const isPlayer = data.players && data.players.some(p => p.userId === currentUserId);

    document.getElementById('holdem-round-bar').textContent = `?쇱슫??${data.round || 0}`;
    document.getElementById('holdem-pot-bar').textContent = `??狩먄?{data.pot || 0}`;

    const communityEl = document.getElementById('holdem-community-cards');
    if (communityEl && data.communityCards) {
      communityEl.innerHTML = data.communityCards.map(c => renderHoldemCard(c)).join('');
    }

    const playersEl = document.getElementById('holdem-players');
    if (playersEl && data.players) {
      playersEl.innerHTML = data.players.map(p => {
        const isMe = p.userId === currentUserId;
        const isTurn = p.userId === data.currentTurn;
        const folded = p.status === 'fold';
        const cardsHtml = (p.cards || []).map(c => renderHoldemCard(c)).join('');
        const nameHtml = !isMe
          ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(p.userId)}')" title="?꾩쟻 蹂닿린">${escapeHTML(p.userId)}</span>`
          : escapeHTML(p.userId) + ' (??';
        return `
          <div class="holdem-player-box ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">
            <div style="display:flex; justify-content:space-between; align-items:center; gap:4px;">
              <div class="holdem-player-name" style="flex:1;">${nameHtml}</div>
              ${p.userId === data.dealerId ? `<div class="holdem-dealer-btn" title="?쒕윭">D</div>` : ''}
            </div>
            <div class="holdem-player-stars">狩먄?{p.stars}</div>
            <div class="holdem-player-status">${folded ? '?뤂截??대뱶' : p.status === 'check' ? '??泥댄겕' : ''}</div>
            <div class="holdem-player-cards">${cardsHtml}</div>
          </div>`;
      }).join('');
    }

    const canAct = isMyTurn && isPlayer && data.phase !== 'waiting' && data.phase !== 'showdown';
    document.getElementById('btn-holdem-check').disabled = !canAct;
    document.getElementById('btn-holdem-fold').disabled = !canAct;

    const holdemGuide = document.querySelector('#holdem-container .poker-hand-guide-grid');
    if (holdemGuide) {
      holdemGuide.querySelectorAll('.poker-hand-item').forEach(el => el.classList.remove('active'));
      const myHand = (data.myHandName || '').trim();
      if (myHand) {
        holdemGuide.querySelectorAll('.poker-hand-item').forEach(el => {
          const nameEl = el.querySelector('.poker-hand-name');
          if (nameEl && nameEl.textContent.trim() === myHand) el.classList.add('active');
        });
      }
    }
  }

  function holdemCheck() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'check' });
  }

  function holdemFold() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'fold' });
  }

  // ?? Seven Poker UI ????????????????????????????????????????????????????????

  function showSevenPokerUI() {
    switchGameView('sevenpoker');
  }

  function renderSevenPoker(data) {
    if (!data) return;

    const isMyTurn = data.currentTurn === currentUserId;
    const isPlayer = data.players && data.players.some(p => p.userId === currentUserId);

    document.getElementById('sevenpoker-round-bar').textContent = `?쇱슫??${data.round || 0}`;
    document.getElementById('sevenpoker-pot-bar').textContent = `??狩먄?{data.pot || 0}`;

    const playersEl = document.getElementById('sevenpoker-players');
    if (playersEl && data.players) {
      playersEl.innerHTML = data.players.map(p => {
        const isMe = p.userId === currentUserId;
        const isTurn = p.userId === data.currentTurn;
        const folded = p.status === 'fold';
        const cardsHtml = (p.cards || []).map(c => {
          if (isMe && c.hidden) {
            let html = renderHoldemCard({ ...c, hidden: false });
            return html.replace('class="holdem-card', 'class="holdem-card my-secret-card');
          } else {
            return renderHoldemCard(c);
          }
        }).join('');
        const nameHtml = !isMe
          ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(p.userId)}')" title="?꾩쟻 蹂닿린">${escapeHTML(p.userId)}</span>`
          : escapeHTML(p.userId) + ' (??';
        return `
          <div class="sevenpoker-player-box ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">
            <div class="sevenpoker-player-name">${nameHtml}</div>
            <div class="sevenpoker-player-stars">狩먄?{p.stars}</div>
            <div class="sevenpoker-player-status">${folded ? '?뤂截??대뱶' : p.status === 'check' ? '??泥댄겕' : ''}</div>
            <div class="sevenpoker-player-cards">${cardsHtml}</div>
          </div>`;
      }).join('');
    }

    const canAct = isMyTurn && isPlayer && data.phase !== 'waiting' && data.phase !== 'showdown' && data.phase !== 'choice';
    document.getElementById('btn-sevenpoker-check').disabled = !canAct;
    document.getElementById('btn-sevenpoker-fold').disabled = !canAct;

    const choiceBox = document.getElementById('sevenpoker-choice-box');
    if (choiceBox) {
      const showChoice = data.phase === 'choice' && isPlayer && !data.myChoiceDone;
      choiceBox.style.display = showChoice ? 'block' : 'none';
      if (showChoice) {
        spChoiceDiscard = -1; spChoiceOpen = -1;
        const me = data.players.find(p => p.userId === currentUserId);
        if (me && me.cards) { spChoiceMyCards = me.cards; renderSpChoiceUI(); }
      }
    }

    const sevenGuide = document.querySelector('#sevenpoker-container .poker-hand-guide-grid');
    if (sevenGuide) {
      sevenGuide.querySelectorAll('.poker-hand-item').forEach(el => el.classList.remove('active'));
      const myHand = (data.myHandName || '').trim();
      if (myHand) {
        sevenGuide.querySelectorAll('.poker-hand-item').forEach(el => {
          const nameEl = el.querySelector('.poker-hand-name');
          if (nameEl && nameEl.textContent.trim() === myHand) el.classList.add('active');
        });
      }
    }
  }

  function sevenpokerCheck() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'check' });
  }

  function sevenpokerFold() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'fold' });
  }

  window.setSpChoice = function(type, idx) {
    if (type === 'discard') { spChoiceDiscard = idx; if (spChoiceOpen === idx) spChoiceOpen = -1; }
    if (type === 'open') { spChoiceOpen = idx; if (spChoiceDiscard === idx) spChoiceDiscard = -1; }
    renderSpChoiceUI();
  };
  window.renderSpChoiceUI = function() {
    const container = document.getElementById('sp-choice-cards');
    if (!container) return;
    container.innerHTML = spChoiceMyCards.map((c, i) => {
      const isDiscard = spChoiceDiscard === i;
      const isOpen = spChoiceOpen === i;
      const style = isDiscard ? 'opacity:0.4; border-color:var(--danger);' : (isOpen ? 'border-color:var(--accent); box-shadow:0 0 8px var(--accent);' : '');
      let cardHtml = renderHoldemCard({...c, hidden: false});
      if (style) cardHtml = cardHtml.replace(/^<div /, '<div style="' + style + '" ');
      return `
        <div style="display:flex; flex-direction:column; gap:6px; align-items:center;">
          ${cardHtml}
          <div style="display:flex; gap:4px;">
            <button onclick="setSpChoice('discard', ${i})" style="padding:4px 6px; font-size:11px; background:${isDiscard?'var(--danger)':'var(--bg-tertiary)'}; color:#fff; border:1px solid var(--border); border-radius:4px; cursor:pointer;">??/button>
            <button onclick="setSpChoice('open', ${i})" style="padding:4px 6px; font-size:11px; background:${isOpen?'var(--accent)':'var(--bg-tertiary)'}; color:#fff; border:1px solid var(--border); border-radius:4px; cursor:pointer;">?몓截?/button>
          </div>
        </div>
      `;
    }).join('');
    const btn = document.getElementById('btn-sp-choice-submit');
    if (btn) btn.disabled = (spChoiceDiscard === -1 || spChoiceOpen === -1);
  };
  window.sendSevenPokerChoice = function() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (spChoiceDiscard === -1 || spChoiceOpen === -1) return;
    sendGameAction({ cmd: 'choice', discardIdx: spChoiceDiscard, openIdx: spChoiceOpen });
    spChoiceDiscard = -1; spChoiceOpen = -1;
  };

  /** ?몃뵒???ъ빱 ?밸? 寃곌낵 ?ㅻ쾭?덉씠 ??4珥덇컙 ?쒖떆 ???먮룞 ?ロ옒 */
  let indianShowdownTimeout = null;
  function showIndianShowdownOverlay(data) {
    if (!data) return;
    const box   = document.getElementById('indian-showdown-box');
    const vsEl  = document.getElementById('indian-showdown-vs');
    const resEl = document.getElementById('indian-showdown-result');
    const overlay = document.getElementById('indian-showdown-overlay');

    const myVal = data.myCard?.value || '?';
    const oppVal = data.opponentCard?.value || '?';
    const mySuit = data.myCard?.suit || '';
    const oppSuit = data.opponentCard?.suit || '';
    const isWin = data.result === 'win';
    const delta = data.heartDelta || 0;
    const deltaStr = delta >= 0 ? `+${delta}` : `${delta}`;

    vsEl.textContent = `??移대뱶 ${myVal}${mySuit}  vs  ?곷? ${oppVal}${oppSuit}`;
    resEl.textContent = isWin ? `?밸━! (${deltaStr} ?섑듃)` : `?⑤같 (${deltaStr} ?섑듃)`;
    resEl.className = 'indian-showdown-result ' + (isWin ? 'win' : 'lose');
    box.className = 'unified-result-box ' + (isWin ? 'win' : 'lose');

    if (indianShowdownTimeout) clearTimeout(indianShowdownTimeout);
    overlay.classList.add('show');
    indianShowdownTimeout = setTimeout(() => {
      overlay.classList.remove('show');
      indianShowdownTimeout = null;
    }, 4000);
  }

  function closeIndianOverlay() {
    const overlay = document.getElementById('indian-showdown-overlay');
    if (overlay) overlay.classList.remove('show');
    if (indianShowdownTimeout) {
      clearTimeout(indianShowdownTimeout);
      indianShowdownTimeout = null;
    }
  }

  /** ?ъ빱(????몃툙?ъ빱) ?쇰떎??寃곌낵 ?ㅻ쾭?덉씠 ??5珥덇컙 ?쒖떆 ???먮룞 ?ロ옒 */
  let pokerShowdownTimeout = null;
  function showPokerShowdownOverlay(data) {
    if (!data) return;
    const winnerEl = document.getElementById('poker-showdown-winner');
    const partEl = document.getElementById('poker-showdown-participants');
    const overlay = document.getElementById('poker-showdown-overlay');

    const winnerId = data.winnerId || '';
    const winningHand = data.winningHand || '';
    const participants = data.participants || [];
    winnerEl.textContent = winningHand ? `?뱀옄: ${winnerId} (${winningHand})` : `?뱀옄: ${winnerId}`;

    let html = '';
    participants.forEach(p => {
      html += `<div class="poker-showdown-row"><span>${escapeHTML(p.userId || '')}</span><span>${escapeHTML(p.handName || '-')}</span></div>`;
    });
    partEl.innerHTML = html || '<div class="poker-showdown-row">??/div>';

    if (pokerShowdownTimeout) clearTimeout(pokerShowdownTimeout);
    overlay.classList.add('show');
    pokerShowdownTimeout = setTimeout(() => {
      overlay.classList.remove('show');
      pokerShowdownTimeout = null;
    }, 5000);
  }

  function closePokerShowdownOverlay() {
    const overlay = document.getElementById('poker-showdown-overlay');
    if (overlay) overlay.classList.remove('show');
    if (pokerShowdownTimeout) {
      clearTimeout(pokerShowdownTimeout);
      pokerShowdownTimeout = null;
    }
  }
</script>
</body>
</html>
