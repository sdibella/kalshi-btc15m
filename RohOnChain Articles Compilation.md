# RohOnChain X Articles Compilation

## Extracted on February 10, 2026

---

# Article 1: The Math Needed for Trading on Polymarket (Complete Roadmap)

**Source:** [https://x.com/RohOnChain/status/2017314080395296995](https://x.com/RohOnChain/status/2017314080395296995)

I'm going to break down the essential math you need for trading on Polymarket. I'll also share the exact roadmap and resources that helped me personally.

Let's get straight to it

A recent research paper just exposed the reality. Sophisticated traders extracted $40 million in guaranteed arbitrage profits from Polymarket in one year. The top trader alone made $2,009,631.76. These aren't lucky gamblers. They're running Bregman projections, Frank-Wolfe algorithms, and solving optimization problems that would make most computer science PhDs uncomfortable.

Bookmark This \-

I'm Roan, a backend developer working on system design, HFT-style execution, and quantitative trading systems. My work focuses on how prediction markets actually behave under load.

When you see a market where YES is $0.62 and NO is $0.33, you think "that adds up to $0.95, there's arbitrage." You're right. What most people never realize is that while they're manually checking whether YES plus NO equals $1, quantitative systems are solving integer programs that scan 17,218 conditions across 2^63 possible outcomes in milliseconds. By the time a human places both orders, the spread is gone. The systems have already found the same violation across dozens of correlated markets, calculated optimal position sizes accounting for order book depth and fees, executed parallel non-atomic trades, and rotated capital into the next opportunity.

The difference isn't just speed. It's mathematical infrastructure.

By the end of this article, you will understand the exact optimization frameworks that extracted $40 million from Polymarket. You'll know why simple addition fails, how integer programming compresses exponential search spaces, and what Bregman divergence actually means for pricing efficiency. More importantly, you'll see the specific code patterns and algorithmic strategies that separate hobby projects from production systems running millions in capital.

Note: This isn't a skim. If you're serious about building systems that can scale to seven figures, read it end to end. If you're here for quick wins or vibe coding, this isn't for you.

---

## Part I: The Marginal Polytope Problem (Why Simple Math Fails)

### The Reality of Multi-Condition Markets

Single condition market: "Will Trump win Pennsylvania?"

- YES: $0.48  
- NO: $0.52  
- Sum: $1.00

Looks perfect. No arbitrage, right?

Wrong.

Now add another market: "Will Republicans win Pennsylvania by 5+ points?"

- YES: $0.32  
- NO: $0.68

Still both sum to $1. Still looks fine.

But there's a logical dependency. If Republicans win by 5+ points, Trump must win Pennsylvania. These markets aren't independent. And that creates arbitrage.

### The Mathematical Framework

For any market with n conditions, there are 2^n possible price combinations. But only n valid outcomes because exactly one condition must resolve to TRUE.

Define the set of valid payoff vectors:

Z \= {φ(ω) : ω ∈ Ω}

Where φ(ω) is a binary vector showing which condition is TRUE in outcome ω.

The marginal polytope is the convex hull of these valid vectors:

M \= conv(Z)

Arbitrage-free prices must lie in M. Anything outside M is exploitable.

For the Pennsylvania example:

1. Market A has 2 conditions, 2 valid outcomes  
2. Market B has 2 conditions, 2 valid outcomes  
3. Combined naive check: 2 × 2 \= 4 possible outcomes  
4. Actual valid outcomes: 3 (dependency eliminates one)

When prices assume 4 independent outcomes but only 3 exist, the mispricing creates guaranteed profit.

### Why Brute Force Dies

NCAA 2010 tournament market had:

- 63 games (win/loss each)  
- 2^63 \= 9,223,372,036,854,775,808 possible outcomes  
- 5,000+ securities

Checking every combination is computationally impossible.

The research paper found 1,576 potentially dependent market pairs in the 2024 US election alone. Naive pairwise verification would require checking 2^(n+m) combinations for each pair.

At just 10 conditions per market, that's 2^20 \= 1,048,576 checks per pair. Multiply by 1,576 pairs. Your laptop will still be computing when the election results are already known.

### The Integer Programming Solution

Instead of enumerating outcomes, describe the valid set with linear constraints.

Z \= {z ∈ {0,1}^I : A^T × z ≥ b}

Real example from Duke vs Cornell market: Each team has 7 securities (0 to 6 wins). That's 14 conditions, 2^14 \= 16,384 possible combinations.

But they can't both win 5+ games because they'd meet in the semifinals.

Integer programming constraints:

Sum of z(duke, 0 to 6\) \= 1 Sum of z(cornell, 0 to 6\) \= 1 z(duke,5) \+ z(duke,6) \+ z(cornell,5) \+ z(cornell,6) ≤ 1

Three linear constraints replace 16,384 brute force checks.

This is how quantitative systems handle exponential complexity. They don't enumerate. They constrain.

### Detection Results from Real Data

The research team analyzed markets from April 2024 to April 2025:

- 17,218 total conditions examined  
- 7,051 conditions showed single-market arbitrage (41%)  
- Median mispricing: $0.60 per dollar (should be $1.00)  
- 13 confirmed dependent market pairs with exploitable arbitrage

The median mispricing of $0.60 means markets were regularly wrong by 40%. Not close to efficient. Massively exploitable.

Key takeaway: Arbitrage detection isn't about checking if numbers add up. It's about solving constraint satisfaction problems over exponentially large outcome spaces using compact linear representations.

---

## Part II: Bregman Projection (How to Actually Remove Arbitrage)

Finding arbitrage is one problem. Calculating the optimal exploiting trade is another.

You can't just "fix" prices by averaging or nudging numbers. You need to project the current market state onto the arbitrage-free manifold while preserving the information structure.

### Why Standard Distance Fails

Euclidean projection would minimize:

||μ \- θ||^2

This treats all price movements equally. But markets use cost functions. A price move from $0.50 to $0.60 has different information content than a move from $0.05 to $0.15, even though both are 10 cent changes.

Market makers use logarithmic cost functions (LMSR) where prices represent implied probabilities. The right distance metric must respect this structure.

### The Bregman Divergence

For any convex function R with gradient ∇R, the Bregman divergence is:

D(μ||θ) \= R(μ) \+ C(θ) \- θ·μ

Where:

- R(μ) is the convex conjugate of the cost function C  
- θ is the current market state  
- μ is the target price vector  
- C(θ) is the market maker's cost function

For LMSR, R(μ) is negative entropy:

R(μ) \= Sum of μ\_i × ln(μ\_i)

This makes D(μ||θ) the Kullback-Leibler divergence, measuring information-theoretic distance between probability distributions.

### The Arbitrage Profit Formula

The maximum guaranteed profit from any trade equals:

max over all trades δ of \[min over outcomes ω of (δ·φ(ω) \- C(θ+δ) \+ C(θ))\] \= D(μ||θ)\*

Where μ\* is the Bregman projection of θ onto M.

This is not obvious. The proof requires convex duality theory. But the implication is clear: finding the optimal arbitrage trade is equivalent to computing the Bregman projection.

### Real Numbers

The top arbitrageur extracted $2,009,631.76 over one year.

Their strategy was solving this optimization problem faster and more accurately than everyone else:

μ \= argmin over μ in M of D(μ||θ)\*

Every profitable trade was finding μ\* before prices moved.

### Why This Matters for Execution

When you detect arbitrage, you need to know:

1. What positions to take (which conditions to buy/sell)  
2. What size (accounting for order book depth)

---

# Article 2: The Math Needed for Trading on Polymarket (Complete Roadmap) \- Part 2

**Source:** [https://x.com/RohOnChain/status/2019131428378931408](https://x.com/RohOnChain/status/2019131428378931408)

I'm going to break down the essential math you need for trading on Polymarket. I'll also share the exact roadmap and resources that helped me personally.

Let's get straight to it

Part 1 reached over 2.2 million views. The response made one thing clear: understanding prediction markets like Polymarket at a system level is essential if you want to make real money like the top arbitrage bots. Theory alone won't get you there. Execution will.

Bookmark This \-

I'm Roan, a backend developer working on system design, HFT-style execution, and quantitative trading systems. My work focuses on how prediction markets actually behave under load. For any suggestions, thoughtful collaborations, partnerships DMs are open.

Before you continue: If you haven't read Part 1, stop here and read it first. Part 1 covers marginal polytopes, Bregman projections, and Frank-Wolfe optimization. This article builds directly on those foundations.

When you understand that Frank-Wolfe solves Bregman projections through iterative optimization, you think "I can implement this." You're right. What most people never realize is that while they're stuck on iteration 1 trying to figure out valid starting vertices, production systems have already initialized with Algorithm 3 (explained in this article), prevented gradient explosions using adaptive contraction, stopped at exactly 90% of maximum profit, and executed the trade. By the time you manually start your first iteration, these systems have already captured the arbitrage across multiple markets, sized positions based on order book depth, and moved on to the next opportunity.

The difference isn't just mathematical knowledge.

It's implementation precision.

By the end of this article, you will know exactly how to build the same system that extracted over $40 million.

Part 1 gave you the theory on integer programming, marginal polytopes, Bregman projections. Part 2 gives you the implementation about how to actually start the algorithm from scratch, prevent the crashes that kill amateur attempts, calculate exact profit guarantees before executing, and why platforms like Polymarket deliberately leave these opportunities open.

The math is public. The $40 million was sitting there. The only difference between the traders who extracted it and everyone else was solving four critical implementation problems first.

This is the exact playbook that turned equations into millions.

Note: This isn't a skim. If you're serious about building systems that can scale to seven figures, read it end to end. If you're here for quick wins or vibe coding, this isn't for you.

---

## Part I: The Initialization Problem (Setting Up Market States)

You can't just start Frank-Wolfe with random numbers and hope it works. The algorithm requires a valid starting point that is a set of vertices from the marginal polytope M that you know are feasible. This is the initialization problem, and it's harder than it sounds.

### Why Initialization Matters

Recall from Part 1 that the Frank-Wolfe algorithm maintains an active set Z\_t of vertices discovered so far. At each iteration, it solves a convex optimization over the convex hull of Z\_t, then finds a new descent vertex by solving an integer program.

But iteration 1 has a problem: what is Z\_0?

You need at least one valid payoff vector to start. Ideally, you need several vertices that span different regions of the outcome space. And critically, you need an interior point u \- a coherent price vector where every unsettled security has a price strictly between 0 and 1\.

Why? Because the Barrier Frank-Wolfe variant we use (covered in Part II of this article) contracts the polytope toward this interior point to control gradient growth. If your interior point has any coordinate exactly at 0 or 1, the contraction fails and the algorithm breaks.

### Algorithm 3: InitFW Explained

The goal of InitFW is threefold:

1. Find an initial set of active vertices Z\_0  
2. Construct a valid interior point u  
3. Extend the partial outcome σ to include any securities that can be logically settled

Here's how it works. You start with a partial outcome σ (the set of securities already settled to 0 or 1\) and iterate through every unsettled security i.

For each security i, you ask the integer programming solver two questions:

- Question 1: "Give me a valid payoff vector z where z\_i \= 1"  
- Question 2: "Give me a valid payoff vector z where z\_i \= 0"

The IP solver either finds such a vector or proves none exists.

**Case 1:** The solver finds valid vectors for both z\_i \= 0 and z\_i \= 1\. This means security i is genuinely uncertain \- it could resolve either way. Add both vectors to Z\_0. Security i remains unsettled.

**Case 2:** The solver finds a vector for z\_i \= 1 but cannot find one for z\_i \= 0\. This means every valid outcome has z\_i \= 1\. Security i must resolve to 1\. Add it to the extended partial outcome σ̂ as (i, 1).

**Case 3:** The solver finds a vector for z\_i \= 0 but cannot find one for z\_i \= 1\. Every valid outcome has z\_i \= 0\. Security i must resolve to 0\. Add it to σ̂ as (i, 0).

After iterating through all unsettled securities, you have:

- An extended partial outcome σ̂ with more securities settled than you started with  
- A set Z\_0 of vertices where each unsettled security appears as both 0 and 1 across different vectors

The interior point u is simply the average of all vertices in Z\_0:

u \= (1/|Z\_0|) × Σ\_{z ∈ Z\_0} z

Because each unsettled security appears as both 0 and 1 in Z\_0, the average guarantees that u\_i is strictly between 0 and 1 for all unsettled i.

### Real Example: NCAA Tournament Initialization

Consider the Duke vs Cornell market from Part 1\. Each team has 7 securities (0 to 6 wins). Initially, no games have been played, so σ \= ∅.

InitFW iterates through all 14 securities:

- For "Duke: 0 wins", the solver finds valid vectors with both 0 and 1 (Duke could win 0 games or not)  
- For "Duke: 6 wins", the solver finds valid vectors with both 0 and 1 (Duke could win the championship or not)  
- Same for all Cornell securities

But there's a constraint: Duke and Cornell can't both reach the finals (5+ wins each), because they'd meet in the semifinals. When InitFW asks for a vector where Duke: 5 wins \= 1 AND Cornell: 5 wins \= 1, the IP solver returns infeasible.

This doesn't settle any securities (both teams could win 5 games, just not simultaneously). But it populates Z\_0 with vectors that respect the dependency. The algorithm now knows the structure of the outcome space before any iterations begin.

After initialization:

- Z\_0 contains valid payoff vectors for all possible tournament outcomes  
- u is a price vector with all coordinates between 0 and 1 (something like 0.14 for each team's 0-6 win probabilities)  
- σ̂ \= σ \= ∅ (no games settled yet)

Once games start settling, subsequent calls to InitFW extend σ̂. If Duke loses in Round 1, the next InitFW call detects that all valid vectors have "Duke: 1+ wins" \= 0, and adds those securities to σ̂. Prices for settled securities lock at 0 or 1 permanently.

### Why This Step Is Critical?

Without proper initialization, Frank-Wolfe can fail in three ways:

**Failure 1: Empty Z\_0** If you don't find any valid vertices, you have nothing to optimize over. The algorithm can't start.

**Failure 2: Boundary interior point** If your interior point u has any coordinate at 0 or 1, the contraction in Barrier Frank-Wolfe is undefined. Gradients explode. The algorithm diverges.

**Failure 3: Missed settlements** If you don't extend σ̂ to include logically settled securities, you waste computation optimizing over prices that should be fixed. And worse you miss arbitrage removal opportunities, because settled securities by definition have no arbitrage.

The Kroer et al. experiments show that InitFW completes in under 1 second for the NCAA tournament even at full size (63 games, 2^63 outcomes).

The IP solver handles these queries efficiently because it's just checking feasibility, not optimizing anything.

Key takeaway: You cannot run Frank-Wolfe without valid initialization. Algorithm 3 solves three problems simultaneously: it constructs a valid starting active set Z\_0, builds an interior point u where all unsettled coordinates are strictly between 0 and 1, and extends the partial outcome σ to include securities that can be logically settled. The IP solver is doing feasibility checks, not optimization, so this step is fast even for enormous outcome spaces. InitFW is what allows Frank-Wolfe to start running, and it's what enables the algorithm to speed up over time as outcomes resolve. Miss this step and your projection either never starts or diverges immediately.

Now let's talk about why that interior point matters so much.

---

## Part II: Adaptive Contraction & Controlled Growth (Why FW Doesn't Break)

Standard Frank-Wolfe assumes the gradient of your objective function is Lipschitz continuous with a bounded constant L. This assumption enables the convergence proof: the algorithm is guaranteed to reduce the gap g(μ\_t) at rate O(L × diam(M) / t)

But LMSR (Logarithmic Market Scoring Rule) violates this assumption catastrophically.

### The Gradient Explosion Problem

Recall from Part 1 that for LMSR, the Bregman divergence is KL divergence:

D(μ||θ) \= Σ\_i μ\_i ln(μ\_i / p\_i(θ))

To minimize this via Frank-Wolfe, we need the gradient ∇D with respect to μ. Taking the derivative:

∇R(μ) \= ln(μ) \+ 1

This gradient is defined only when μ \> 0\. As any coordinate μ\_i approaches 0, the gradient component (∇R)\_i \= ln(μ\_i) \+ 1 goes to negative infinity.

This is a problem. The marginal polytope M has vertices where some coordinates are exactly 0\. Securities that resolve to False have μ\_i \= 0 in the corresponding payoff vector. As Frank-Wolfe iterates approach these boundary vertices, the gradient explodes.

Standard FW convergence analysis requires a bounded Lipschitz constant. Here, L is unbounded. The algorithm could diverge, oscillate, or get stuck.

The Kroer et al. solution: Barrier Frank-Wolfe with adaptive contraction.

### Controlled Growth Condition

Instead of requiring a bounded Lipschitz constant globally, we only require it locally on contracted subsets of M.

Define the contracted polytope:

M' \= (1 \- ε)M \+ εu

where ε ∈ (0,1) is the contraction parameter and u is the interior point from InitFW.

Geometrically, M' is the polytope M shrunk toward the point u. Every vertex v of M gets pulled toward u:

v' \= (1 \- ε)v \+ εu

Because u has all coordinates strictly between 0 and 1 (by construction in InitFW), and ε \> 0, every coordinate of v' is strictly between 0 and 1\. The contracted polytope M' stays away from the boundary.

Now the gradient ∇R(μ) is bounded on M'. Specifically, the Lipschitz constant on M' is:

L\_ε \= O(1/ε)

The smaller ε (closer to the true polytope M), the larger L\_ε. But critically, L\_ε is finite for any fixed ε \> 0\.

This gives us a trade-off:

- Large ε: Fast convergence (small L\_ε), but we're optimizing over the wrong polytope (far from M)  
- Small ε: Slow convergence (large L\_ε), but we're optimizing over something close to M

The adaptive contraction scheme solves this by starting with large ε and decreasing it over time.

### The Adaptive Epsilon Rule

Algorithm 2 in the Kroer paper implements this. At each iteration t, the algorithm maintains ε\_t and updates it according to:

If g(μ\_t) / (-4g\_u) \< ε\_{t-1}: ε\_t \= min{g(μ\_t) / (-4g\_u), ε\_{t-1} / 2} Else: ε\_t \= ε\_{t-1}

---

# Article 3: Why Prediction Markets Aren't Gambling? (The Math)

**Source:** [https://x.com/RohOnChain/status/2020565633453412751](https://x.com/RohOnChain/status/2020565633453412751)

I'll show you the exact structural difference between betting and trading on prediction markets. This is the system that turns Polymarket from gambling into a repeatable, extractable edge.

Let's get straight to it.

Part 1 and Part 2 of the math series crossed millions of impressions. The message is clear. People understand there's a fundamental difference. Most just don't know what it is yet. This article fixes that.

This is not Part 3 of the math series. Part 3 is in development with intense research and will cover the execution infrastructure that scales to seven figures.

Before we get there, you need to understand what you're actually executing.

Bookmark This \-

I'm Roan, a backend developer working on system design, HFT-style execution, and quantitative trading systems. My work focuses on how prediction markets actually behave under load. For any suggestions, thoughtful collaborations, partnerships DMs are open.

## The Framework You'll Implement right now

Before anything else, here's the five point diagnostic that tells you if you're gambling or trading. Run this on your 20 positions:

### Test 1: Exit Before Resolution

For each trade, ask: did I close this position before the event resolved?

- If YES \<50%: You're gambling on outcomes  
- If YES \>80%: You're trading probability drift

### Test 2: Median Hold Time

Calculate median time between entry and exit across all trades.

- If \>24 hours: You're waiting for outcomes (gambling)  
- If \<6 hours: You're capturing information flow (trading)

### Test 3: Position Size Consistency

Do your position sizes correlate with edge magnitude?

- If you bet the same amount every time: Gambling  
- If size scales with calculated edge: Trading

### Test 4: Order Type Distribution

What percentage of your orders are limit orders vs market orders?

- If \<30% limit orders: You're getting adversely selected  
- If \>90% limit orders: You're avoiding informed flow

### Test 5: Profit Source

Where do profits come from?

- If from being right about outcomes: Gambling (not scalable)  
- If from structural mispricing: Trading (scalable)

Run these five tests right now on your trade history. The results tell you everything about whether you're building a system or playing a game.

I'll wait. Seriously, if you haven't run these tests before reading further, you're missing the point.

Done? Good. Now let's talk about why these metrics matter and what the research actually shows.

---

## What the Research Revealed

Between 1988 and 2004, researchers ran prediction markets alongside traditional polls for US Presidential elections. They analyzed 964 polls across five election cycles and compared accuracy.

Markets beat polls 74% of the time.

But here's what matters: the accuracy gap widened dramatically for forecasts made 100+ days before the election. Far from the event, markets significantly outperformed expert polling.

Why? Markets don't aggregate opinions. They aggregate information weighted by capital deployment.

Someone with real information bets large. Someone guessing bets small or sits out. Over time, capital concentrates in informed hands through pure selection pressure. Winning traders compound. Losing traders exit.

The price reflects the beliefs of traders who have been consistently right, weighted by how much capital they control. This is not consensus.

This is Darwinian information filtering.

A poll asks everyone their opinion and averages the result. A market forces everyone to put capital at risk and lets selection pressure filter truth from noise.

That's the fundamental difference.

---

## The Three Games Being Played Simultaneously

I've analyzed thousands of traders across Polymarket and other major Prediction markets. Three distinct patterns emerge.

### Pattern 1: Outcome Betting

**Trader profile:** Holds positions for 3 to 7 days. Uses market orders. Sizes positions based on conviction ("I'm really sure"). Exits at resolution.

**Profitability:** Negative or break even after fees.

**Why:** Every trade is adversely selected. When you market buy, someone with better information is happy to sell. You're the exit liquidity.

### Pattern 2: Information Trading

**Trader profile:** Holds positions for 6 to 18 hours. Trades around news flow. Exits when information is priced in. Sizes based on information edge.

**Profitability:** Positive but inconsistent. Depends on information access.

**Why:** Information edges are temporary and competitive. Once your edge is gone, so is your profit.

### Pattern 3: Structural Exploitation

**Trader profile:** Holds positions for 2 to 6 hours. Uses limit orders exclusively. Sizes with Kelly criterion. Trades arbitrage and mispricing.

**Profitability:** Consistently positive. Top performer made $2.01M in one year.

**Why:** Structural edges are renewable. Arbitrage exists because the market chose speed over perfect accuracy. The edge regenerates continuously.

Here's what separates these three: Pattern 1 and 2 depend on being right. Pattern 3 depends on math being right.

You can't control whether your prediction is correct. You can control whether your execution captures structural inefficiency.

The top trader executed 4,049 trades in one year.

- Average profit per trade: $496.  
- Win rate: \>90%.

They weren't predicting outcomes. They were solving integer programs faster than competitors.

Let me explain what that means.

---

## The Arbitrage That's Happening Right Now

Polymarket uses a Central Limit Order Book. Orders match when bid meets ask. Simple.

But here's what creates opportunity: the CLOB doesn't enforce mathematical relationships in real time.

Example: YES trades at $0.62 NO trades at $0.33 Sum: $0.95

Mathematically, YES \+ NO must equal $1.00 because exactly one outcome resolves TRUE. But in the CLOB, prices can temporarily violate this.

Why? Because enforcing the constraint requires running optimization algorithms on every trade. That takes time. Polymarket chose execution speed over mathematical perfection.

**Result:** buy YES at $0.62 and NO at $0.33. Total cost: $0.95. Guaranteed payout: $1.00. Guaranteed profit: $0.05 per complete set.

Research analyzing 2024 Polymarket data found 41% of multi outcome markets showed this mispricing at some point during their lifecycle. Median mispricing: $0.08 per dollar.

The opportunities lasted minutes to hours.

Fast traders captured them. Slow traders provided liquidity.

But here's what makes this interesting from a quant perspective: this isn't inefficiency that goes away. It's structural. It's the cost Polymarket pays for having fast markets.

They could eliminate arbitrage by using LMSR (Logarithmic Market Scoring Rule). LMSR mathematically guarantees arbitrage free pricing through a convex cost function that enforces all outcome probabilities sum to 1\.

But LMSR requires solving optimization problems on every trade. Execution latency increases from 50ms to 30 seconds or more.

Users leave. Liquidity dies. Platform fails.

So the arbitrage stays. By design.

For you, this means: the edge is renewable. It's not alpha decay. It's structural friction that exists as long as the platform prioritizes speed.

The question becomes: can you execute fast enough to capture it?

---

## The Latency Stack (Why 30ms Matters)

When news drops that Trump leads in a swing state poll, what happens?

### Retail trader flow:

See news on Twitter (30 seconds delay) Open Polymarket (5 seconds) Check current price (3 seconds) Decide to buy (10 seconds) Click buy, confirm transaction (8 seconds) Transaction submits to blockchain (2 seconds) **Total: 58 seconds from news to execution.**

### Professional system flow:

News API webhook fires (500ms) NLP model extracts signal (200ms)

---

*End of Compilation*  
