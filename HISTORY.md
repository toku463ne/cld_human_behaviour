# 変更の履歴

**今の操作方法は `README.md`、設計とルールは `CLAUDE.md` にあります。** この文書は「いつ何が変わったか」だけを残します。

README.md は常に最新の状態だけを書く方針なので、そこから落ちた記述（廃止した操作、変わったコマンド、以前の既定値）はここに移します。新しい項目は**末尾に追記**し、過去の項目は書き換えません。

「なぜそう決めたか」は `CLAUDE.md` の「設計判断ログ」にあります。この文書は結果として何がどう変わったかの記録です。

---

## 2026-08-12 engine と cmd/devview の最初の版

`human_evolution_sim.html`（v0のブラウザ版プロトタイプ）をGoに移植。`engine` パッケージと、それを直接importして描画する `cmd/devview` を追加した。

- 起動は `go run ./cmd/devview` のみ。オプションは無し
- 操作はスペース（一時停止）とクリック（ノード選択）だけ。ウィンドウは900x650で、世界がウィンドウ全体を占めていた
- クリックで見えるのは血統と他者への強さ推定（推定値±標準偏差・リスク・観測数・真値）

## 2026-08-12 実装ロードマップ 段階0〜6

状態モデルを 体力・空腹度・食料 の3軸に置換し、能力を パワー／理性／頭の良さ に分離。トリガー方式の意思決定、効用比較、継続戦闘と逃走、記憶と強さ推定、先制排除、頭の良さによる戦略選択までを実装。

- `DefaultConfig` を「食料が足りない世界」に振り直した（`FoodSpawnRate` 1.5→0.20相当）。食料が余っていると争いも餓死も起きず、能力に選択圧がかからないため
- ベンチマークの初期値: `BenchmarkEngineStep` 300体で約283µs/op、`BenchmarkEngineStepCrowded` 約633µs/op、`BenchmarkDecide` 約5.3µs/op

## 2026-08-13 意思決定トレースと、1ノードを追うための操作

`Decide()` 1回分（トリガー・比較した候補と効用の内訳・選ばれた行動）を記録できるようにし、devview で1ノードを追えるようにした。

- `engine`: `World.TrackDecisions(id, on)` / `DecisionTraces(id)` / `LastDecisionTrace(id)` を追加。記録は指定したノードだけ
- `engine`: 効用式の実装を `Utility`（目的ごとの `Goal{Value, Chance}` ＋リスク・体力コスト・時間コスト）に一本化した。表示用の別計算は作っていない
- `engine`: 再判断のきっかけに名前が付いた（`Trigger`）。それまでは bool 1個で理由を捨てていた
- `cmd/devview`: ウィンドウを 1280x660 に拡張し、右に幅460の固定パネルを追加。世界の描画領域は 820x660 になった（それ以前は世界がウィンドウ全体）
- `cmd/devview`: 操作を追加 — `→`/`n`（1tick進める）、`-`/`=`（速度5段階）、`[`/`]`（決定履歴）、Tab（トレース／強さ推定の切替）、Esc（選択解除）。クリックでの選択は「選択＝追跡」に変わった
- `cmd/devview`: 起動オプション `-follow` / `-slow` / `-seed` を追加
- `engine/decision_test.go` を新設（状況を組み立てて `Decide()` を1回呼び、選ばれる行動をassertするシナリオテスト）
- 速度への影響: `BenchmarkDecide` 5.36µs→5.77µs（+7.5%）、`BenchmarkEngineStep` 276µs→283µs。`Utility` 構造体の組み立て分で、追跡していないノードでも払う。同一シード5000tickで統計値が完全一致することは確認済み

## 2026-08-13 README.md と HISTORY.md を分けた

操作方法を `README.md` に集約し、この文書（`HISTORY.md`）を履歴の置き場にした。`CLAUDE.md` の「現在の機能」にあった devview の起動方法・キー操作・画面の読み方は README.md に移した（同じことを2箇所に書かないため）。

## 2026-08-14 A/B実験ランナーと、戦略深さゲートの既定オフ化

ルール変更が能力の選択圧をどちらに動かすかを測るため、ヘッドレスの `cmd/experiment` を追加した。その最初の測定結果として、頭の良さの「戦略の幅」の経路（深さゲート）を既定で無効にした。

- `cmd/experiment/` を新設。N条件 × Nシード × Ntick を並列で回し、条件ごとの平均±標準誤差と、**同じシードどうしの差分**（paired difference）を表示する。`-csv` で推移の時系列も出る。使い方は README.md
- `engine`: `Config.StrategyDepthUnlock = 0` が「ゲートなし（全戦略解禁）」を意味するようになった。ゼロ除算を避ける `strategyDepth()` を経由する
- `engine`: **`DefaultConfig` の `StrategyDepthUnlock` を 16 → 0 に変更**（既定で深さゲートが無効になった）。頭の良さは評価ノイズ（`ChoiceNoise`）だけを通して効く
- 測定値（24シード × 20000tick、同一シードの差分）:
  - `nogate` − `baseline`: 平均知能の上がり幅 **+4.52 ± 0.95**、戦闘 +3589、殺害 +23、餓死 −22、個体数 −2.0 ± 2.1（変化なし）
  - `gate20`（閾値を20/40/60に）− `baseline`: 平均知能の上がり幅 **−3.87 ± 0.79**（知能への選択圧が負に反転する）
  - `noquality`（`ChoiceNoise = 0`）− `baseline`: 知能の上がり幅 −1.71 ± 0.81。ゲートだけでは知能はほぼ選択されない（+0.63 ± 0.51）
  - 60000tick × 16シードでも向きは同じ（`nogate` +4.86 ± 2.29）
- 速度への影響: `BenchmarkDecide` 5.8µs → 8.0µs（**+36%**）。ゲートが無くなり全個体が観察・攻撃まで採点するようになったぶん。`BenchmarkEngineStep` は同時測定で 382µs（ゲート有）対 357µs（無）で、差は測定ノイズの範囲
- `engine/world_test.go` の `TestIntelligenceGatesTheStrategiesAvailable` は明示的に `StrategyDepthUnlock = 16` を設定するようになった（既定がオフになったため）。`engine/decision_test.go` に中段の閾値とオフスイッチのテストを追加

## 2026-08-14 集団化の指標と、ルール・パラメータの文書を分離

協力・外敵を入れる前段として、**群れているかどうかを測る指標**を追加し、外敵を入れる前のベースラインを取った。あわせてルールとパラメータの記述を CLAUDE.md から独立した文書に切り出した。

- `engine`: `World.Spacing()` を追加。`AvgNeighbours`（視界内の他個体数）／`AvgNearestDist`（最近傍までの距離）／`Clumping`（同じ個体数を一様にばらまいた場合との比）を返す。O(n^2) なので `Stats()` とは別にしてある（`Stats()` は毎フレーム読まれる）
- `cmd/experiment`: 指標 `clumping` / `neighbours` / `nearest` / `killShare`（死因に占める殺害の割合）を追加。条件 `morefood`（`FoodSpawnRate` 0.30）と `scarce`（0.12）を追加
- **`NODE.md` / `ENEMY.md` / `PARAMETERS.md` を新設。** CLAUDE.md の「シミュレーションのルール」「設計中のルール」の詳細はここへ移し、CLAUDE.md 側には「崩してはいけない原則」と将来実装だけを残した
- ベースライン測定（24シード × 20000tick）:
  - `clumping` 1.19、`neighbours` 18.1、`nearest` 21.2、`killShare` 0.61
  - 食料供給を 0.12〜0.30 で振っても `killShare` は 0.59〜0.64 でほとんど動かない。**一方で殺害の絶対数は食料が増えるほど増える**（個体数が増えて遭遇が増えるため）。「食料を増やせば争いが減る」は成り立たない
  - 能力の選択圧（`dPower` +8.3、`dIntelligence` +6.9）は食料供給に対して頑健。`dRationality` だけが +0.5〜1.0 と一桁小さい
