(function () {
  if (window.parent !== window && new URLSearchParams(location.search).get("embedded") === "1") {
    const exitButton = document.getElementById("exitGuide");
    if (exitButton) exitButton.style.visibility = "hidden";
  }
  const locale = new URLSearchParams(location.search).get("locale");
  if (locale === "en") {
    const translations = [
      [
        "调研近一年企业级 AI Agent 的落地情况，重点看产品与厂商、行业案例、技术瓶颈三部分。输出一份 1500 字以内的简报，附来源链接；不确定的地方标注「待核实」。",
        "Research enterprise AI agent adoption over the past year, focusing on products and vendors, industry cases, and technical limitations. Write a brief of no more than 1,500 words with source links, and mark uncertain details as “to be verified.”",
      ],
      ["把你的 AI 团队，", "Your AI team,"],
      ["装进一台电脑", "on your computer"],
      [
        "CSGClaw 是跑在本机的多智能体协作平台。这份导览用 10 个步骤，带你把第一个任务、第一次协作和第一个定时任务真正跑起来。",
        "CSGClaw is a local multi-agent collaboration platform. This 10-step guide will help you launch your first task, collaborate with a team, and schedule a task.",
      ],
      ["约 8 分钟", "About 8 minutes"],
      ["10 个步骤", "10 steps"],
      ["开始导览", "Start guide"],
      ["适用 CSGClaw 0.9+", "For CSGClaw 0.9+"],
      ["我的智能体", "My agents"],
      ["主理人 Manager", "Manager"],
      ["通用 · 拿不准就找它", "General purpose · Start here if unsure"],
      ["产品经理 Alice", "Product Manager Alice"],
      ["需求 · 原型 · 验收", "Requirements · Prototypes · Reviews"],
      ["高级开发工程师", "Senior Software Engineer"],
      ["前端 · 接口 · 调试", "Frontend · APIs · Debugging"],
      ["行业研究分析师", "Industry Research Analyst"],
      ["调研 · 证据 · 简报", "Research · Evidence · Briefs"],
      ["在线", "Online"],
      ["软件开发 Team", "Software Development Team"],
      ["5 名成员 · 可协作", "5 members · Ready to collaborate"],
      ["本地运行 · 文件在你手里", "Runs locally · Your files stay yours"],
      ["新建任务 · 选择智能体", "New task · Choose an agent"],
      ["新手篇", "Getting started"],
      ["进阶篇", "Next level"],
      ["第 ", "Step "],
      [" 步", ""],
      ["先选对入口：", "Choose the right starting point:"],
      ["任务、协作任务、定时任务", "Tasks, team tasks, and scheduled tasks"],
      [
        "CSGClaw 只有三个开始工作的入口。跟一个智能体干，是任务；几个智能体组队，是协作任务；到点开始，是定时任务。",
        "CSGClaw has three ways to get work done. Work with one agent on a task, bring several agents together for a team task, or schedule a task to run later.",
      ],
      ["任务", "Task"],
      [
        "（单智能体）：选一个智能体，把事说清楚，回车。",
        "(single agent): Choose an agent, describe the work, and press Enter.",
      ],
      ["协作任务", "Team task"],
      [
        "：在执行者里选一个团队，主理人 Manager 拆任务、分给成员，你只看结论。",
        ": Choose a team. Manager breaks down the work and assigns it to members; you review the results.",
      ],
      ["定时任务", "Scheduled task"],
      [
        "：设好时间与提示词，到点自动生成一次可追踪的任务。",
        ": Set a time and prompt to create a trackable task automatically.",
      ],
      ["你可以这样说", "Try saying"],
      [
        "“先帮我把这件事拆成三步，确认计划后再开始。”",
        "“Break this down into three steps. Show me the plan before you start.”",
      ],
      ["下一步", "Next"],
      ["智能体 · 我的智能体与团队", "Agents · My agents and teams"],
      ["智能体不是聊天窗口，", "Agents are more than chat windows."],
      ["是可以养成的同事", "They become teammates you can shape."],
      [
        "每个智能体都有自己的指令、模型、工作目录、技能、连接器和记忆。你把一次成功的做法留在它身上，下一次就不用从头解释。",
        "Each agent has its own instructions, model, working directory, skills, connectors, and memory. Teach it what works once, and you will not need to explain it all again.",
      ],
      [" 是通用智能体，拿不准选谁时就选它。", " is your general-purpose agent. Choose it when you are unsure."],
      ["领域智能体", "Specialized agents"],
      [
        "按职责拆：产品、工程、测试、调研、写作各管一段。",
        "Divide work by role: product, engineering, testing, research, or writing.",
      ],
      ["团队", "Teams"],
      [
        "把常用的几个智能体固定成一组，下次直接点名。",
        "Save frequently used agents as a team and call on them together next time.",
      ],
      [
        "“你是一名资深行业研究分析师，先给结论，再给证据和来源。”",
        "“Act as a senior industry research analyst. Start with the conclusion, then provide evidence and sources.”",
      ],
      ["任务说清楚的四个部分", "Four parts of a clear task"],
      ["建议模板", "Suggested template"],
      ["01 · 目标", "01 · Goal"],
      ["要解决什么问题，最后交付什么", "What problem to solve and what to deliver"],
      ["02 · 范围", "02 · Scope"],
      ["看哪段时间、哪些来源、哪些不谈", "Time period, sources, and exclusions"],
      ["03 · 产出", "03 · Deliverable"],
      ["文件、网页、PPT、表格，写清格式", "Specify the format: document, web page, slides, or spreadsheet"],
      ["04 · 约束", "04 · Constraints"],
      ["字数、语气、来源偏好、不能做什么", "Length, tone, source preferences, and limitations"],
      ["Enter 发送", "Enter to send"],
      ["把目标、范围、产出、约束一次说清", "Specify the goal, scope, deliverable, and constraints"],
      ["任务说清楚，", "Be clear about the task:"],
      ["比选哪个模型更重要", "it matters more than choosing a model"],
      ["智能体不会读心。把", "Agents cannot read your mind. Specify the "],
      [
        "四件事写全，结果会稳定得多；长任务还可以先让它复述计划，再开工。",
        " in full for more consistent results. For longer tasks, ask the agent to repeat the plan before starting.",
      ],
      ["目标、范围、产出、约束", "goal, scope, deliverable, and constraints"],
      ["给它一个", "Give it a "],
      ["可验收的产出", "verifiable deliverable"],
      [
        "：一份文件、一个网页、一张表，而不是“帮我看看”。",
        ": a document, web page, or spreadsheet instead of “take a look.”",
      ],
      ["告诉它", "Tell it "],
      ["不要做什么", "what not to do"],
      ["，比只说要做什么更有效。", "; this can be more useful than listing only what to do."],
      ["不确定的地方，要求它", "Ask it to "],
      ["标注出来", "flag uncertain details"],
      ["，而不是猜。", " instead of guessing."],
      [
        "“先复述你的计划，我确认后再开始；不确定的地方标注出来。”",
        "“Repeat your plan before starting, and flag anything uncertain.”",
      ],
      ["设置 · 技能", "Settings · Skills"],
      ["缺什么能力，", "Give it the capabilities"],
      ["就给它装什么", "it needs"],
      ["技能", "Skills"],
      ["是做事的方法，", " are ways to do work; "],
      ["连接器", "connectors"],
      [
        "是外部系统的入口。技能装在本机或从技能市场取用；连接器独立保存凭据，每个智能体按需启用。",
        " connect external systems. Install skills locally or from the skills marketplace. Connectors store credentials separately and can be enabled for each agent.",
      ],
      [
        "内置文档、表格、PDF、PPT、OCR、前端设计等常用技能。",
        "Built-in skills cover documents, spreadsheets, PDF, slides, OCR, frontend design, and more.",
      ],
      ["飞书、知识库等外部能力，通过连接器接入。", "Connect external services such as Feishu and knowledge bases."],
      ["凭据只存在连接器里，", "Credentials stay in connectors. "],
      ["不要写进提示词", "Never put them in prompts"],
      [
        "“用 docx 技能把这份纪要整理成 Word，按公司模板排版。”",
        "“Use the docx skill to format these meeting notes as a Word document using our company template.”",
      ],
      ["定时任务 · 新建", "Scheduled task · Create"],
      ["让重复发生的事，", "Automate recurring work"],
      ["自己开始", "to run on its own"],
      [
        "定时任务不是提醒，而是一次真正的执行：到点把提示词发给指定智能体，并生成一条",
        "A scheduled task does more than remind you. At the set time, it sends a prompt to an agent and creates a ",
      ],
      ["可追踪的任务记录", "trackable task record"],
      ["。", "."],
      ["支持", "Supports "],
      ["不重复、每天、每周、每月", "once, daily, weekly, or monthly"],
      ["，也可以设到期日。", ", with an optional end date."],
      ["指派给一个智能体，写清每次要它做什么。", "Assign it to an agent and describe what to do each time."],
      [
        "执行结果进入任务历史，可以回看、继续追问。",
        "Results appear in task history, where you can review them and follow up.",
      ],
      [
        "“每周一 9:00，汇总上周的项目进展，输出一页周报。”",
        "“Every Monday at 9:00, summarize last week’s project progress in a one-page report.”",
      ],
      ["进入进阶篇", "Continue to next level"],
      ["协作任务 · 企业级 Agent 调研", "Team task · Enterprise AI agent research"],
      ["进行中 3 / 4", "3 / 4 in progress"],
      ["产品与厂商", "Products and vendors"],
      ["4 条主线 · 已交付", "4 workstreams · Delivered"],
      ["行业案例与价值", "Industry cases and value"],
      ["6 个场景 · 已交付", "6 scenarios · Delivered"],
      ["技术瓶颈与风险", "Technical limits and risks"],
      ["正在检索来源", "Searching sources"],
      ["汇总与冲突标注", "Synthesis and conflicting findings"],
      ["依赖前三项", "Waiting for the first three"],
      ["任务", "Tasks"],
      [
        "已拆成 4 个子任务，三路并行检索；最后统一汇总，冲突的地方单独标注。",
        "I split this into four subtasks. Three are running in parallel; I will synthesize the results and flag conflicts.",
      ],
      ["研究分析师", "Research Analyst"],
      ["产品经理", "Product Manager"],
      [
        "产品线梳理完成，整理出 12 条关键发布与时间点。",
        "The product landscape is mapped, with 12 key launches and dates.",
      ],
      [
        "发现两家厂商对“落地”的口径不一致，已记录，等待汇总时统一。",
        "Two vendors define “deployment” differently. I noted this for the final synthesis.",
      ],
      ["先按这个结构继续，汇总时给我一页结论。", "Keep going with this structure and give me a one-page summary."],
      ["输入消息，@ 引用成员或文件", "Message, @mention a member or file"],
      ["发送 ↑", "Send ↑"],
      ["一个人干不完的活，", "When one agent cannot do it alone,"],
      ["交给一个团队", "bring in a team"],
      ["在新建任务时把一个", "Choose a "],
      ["团队", "team"],
      [
        "选为执行者，这次任务就变成协作任务：主理人 Manager 拆解、成员并行、结果汇总。你不需要分别开五个聊天窗口。",
        " as the assignee when creating a task. Manager will break down the work, members will work in parallel, and the results will be combined. You do not need to open five separate chats.",
      ],
      ["任务看板", "The task board"],
      ["显示依赖与进度，谁卡住了一眼能看到。", "shows dependencies and progress so you can see what is blocked."],
      ["成员各自有独立上下文，不会互相污染。", "Each member has a separate context."],
      [
        "你只在关键节点做决定，其余交给主理人 Manager 推进。",
        "Make decisions at key points and let Manager handle the rest.",
      ],
      [
        "“分三路并行调研，最后由主理人 Manager 汇总，标注冲突和不确定项。”",
        "“Research this in three parallel tracks, then have Manager synthesize the findings and flag conflicts and unknowns.”",
      ],
      ["设置 · 模型服务", "Settings · Model services"],
      ["模型是你的选择，", "Choose your models"],
      ["不是产品的绑定", "without being locked in"],
      [
        "可以同时接入 OpenCSG、DeepSeek、Codex、Claude Code 等服务商；每个智能体都能单独配置模型，图片与视频模型也可以分开指定。",
        "Connect providers such as OpenCSG, DeepSeek, Codex, and Claude Code. Configure a model for each agent, with separate models for images and video.",
      ],
      ["新智能体默认使用你设置的", "New agents use your "],
      ["默认模型", "default model"],
      ["。", "."],
      [
        "同一个团队里，可以让强模型做判断、快模型做整理。",
        "Use a more capable model for decisions and a faster one for routine work within the same team.",
      ],
      [
        "切换服务商不会影响已有任务与工作目录里的文件。",
        "Changing providers does not affect existing tasks or files in your working directory.",
      ],
      [
        "“先用 deepseek-flash 跑通流程，关键评审再换更强的模型。”",
        "“Use deepseek-flash to get the workflow running, then switch to a stronger model for key reviews.”",
      ],
      ["本机目录", "Local folder"],
      ["调研简报.md", "Research brief.md"],
      ["方案对比.html", "Options comparison.html"],
      ["工作目录", "Working directory"],
      ["任务在这里读写文件，产出直接落盘。", "Tasks read and write files here; outputs are saved directly."],
      ["✓ 默认目录", "✓ Default folder"],
      ["项目规则", "Project rules"],
      ["放一份 AGENTS.md，让智能体先读项目约定。", "Add an AGENTS.md so agents can read your project conventions."],
      ["✓ 可选", "✓ Optional"],
      [
        "文件属于你，不属于任务记录。任务结束后仍然在目录里。",
        "Your files belong to you, not to a task record. They stay in the folder when the task ends.",
      ],
      ["文件留在你的电脑上，", "Your files stay on your computer,"],
      ["产出看得见", "so you can see the results"],
      ["CSGClaw 跑在本机，智能体在", "CSGClaw runs locally. Agents work in a "],
      ["真实的工作目录", "real working directory"],
      [
        "里读写文件。新建任务时可以切换目录，产出直接留在项目里。",
        " to read and write files. Choose a folder when creating a task, and outputs stay in your project.",
      ],
      ["任务输入框左下角可以切换", "Change the "],
      ["工作目录", "working directory"],
      ["。", "."],
      ["运行时支持", "Supported runtimes include "],
      ["等，统一在设置里管理。", "; manage them in Settings."],
      ["产出直接落在目录里，不锁在任务记录中。", "Outputs are saved in the folder, not locked in task history."],
      ["在项目里放一份", "Add an "],
      ["，给智能体一份项目规则。", " to your project to share its conventions with agents."],
      [
        "“在这个项目目录里改，改完把改动列给我看。”",
        "“Make the changes in this project folder and list them when you are done.”",
      ],
      ["智能体模板 · 12", "Agent templates · 12"],
      ["官方", "Official"],
      ["我的", "My templates"],
      ["文档处理专家", "Document Specialist"],
      ["Word · PDF · 排版", "Word · PDF · Formatting"],
      ["检索 · 证据 · 简报", "Research · Evidence · Briefs"],
      ["页面 · 组件 · 调试", "Pages · Components · Debugging"],
      ["前端工程师", "Frontend Engineer"],
      ["产品经理 Alice", "Product Manager Alice"],
      ["需求 · 原型 · 验收", "Requirements · Prototypes · Reviews"],
      ["测试工程师", "QA Engineer"],
      ["用例 · 回归 · 报告", "Test cases · Regression · Reports"],
      ["＋ 新建模板", "+ New template"],
      [
        "把顺手的智能体另存为模板，指令、技能和记忆摘要一起带走。",
        "Save a useful agent as a template, including its instructions, skills, and memory summary.",
      ],
      ["智能体模板", "Agent templates"],
      ["把一次成功，", "Turn a success"],
      ["变成下一次的起点", "into a starting point for next time"],
      [
        "智能体可以另存为模板，团队可以保留成员配置，技能可以带到下一个项目，定时任务可以复制节奏。用得越久，你的团队越像你的团队。",
        "Save agents as templates, reuse team setups, bring skills to new projects, and copy scheduled workflows. Your team becomes more yours over time.",
      ],
      ["官方模板可直接启用，也可以从", "Start with an official template or choose one from "],
      ["我的模板", "My templates"],
      ["开始。", "."],
      ["模板包含指令、技能与", "Templates include instructions, skills, and "],
      ["只读记忆摘要", "a read-only memory summary"],
      ["。", "."],
      ["团队配置可复用，不必每次重新点名。", "Reuse team setups without assigning members again each time."],
      [
        "“把这个智能体保存为模板，下次新项目直接启用。”",
        "“Save this agent as a template so I can use it in my next project.”",
      ],
      ["最后一步", "Finish"],
      ["现在，开始你的", "Now, start your"],
      ["第一个 30 分钟", "first 30 minutes"],
      [
        "不用一次学完。按下面四步在应用里走一遍，你就掌握了 CSGClaw 的主要用法；剩下的，交给你的团队。",
        "You do not need to learn everything at once. Try these four steps in the app to get started with CSGClaw, then let your team take it from there.",
      ],
      ["让主理人 Manager 做一件小事", "Ask Manager to do one small thing"],
      ["整理一份资料，或写一封邮件。", "Organize some notes or draft an email."],
      ["装一个你常用的技能", "Install a skill you use often"],
      ["docx、pdf，或者前端设计。", "Try docx, PDF, or a frontend design skill."],
      ["设一个定时任务", "Create a scheduled task"],
      ["每天汇总进展，或每周看一眼竞品。", "Summarize progress daily or review competitors weekly."],
      ["把顺手的智能体存成模板", "Save a useful agent as a template"],
      ["下次新项目，直接启用。", "Use it directly in your next project."],
      [
        "先从第一步开始，做完一件小事，剩下的自然会用起来。",
        "Start with one small task. The rest will come naturally.",
      ],
      ["团队已经在这里，", "Your team is ready."],
      ["剩下的只是第一句话。", "All that is left is your first prompt."],
      [
        "回到应用后，从「新建任务」开始。拿不准选谁，就选主理人 Manager。",
        "Back in the app, start with “New task.” If you are unsure which agent to choose, pick Manager.",
      ],
      ["退出导览", "Exit guide"],
      ["重新看一遍", "Replay guide"],
      ["封面 · 开始之前", "Cover · Before you begin"],
      ["封面", "Cover"],
      ["第 ", "Step "],
      ["步", ""],
      [" · ", " · "],
      ["开始", "Start"],
      ["你可以关闭当前标签页了", "You can close this tab now."],
      ["步骤导航", "Step navigation"],
      ["上一步", "Previous step"],
    ];
    translations.sort((a, b) => b[0].length - a[0].length);
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let textNode;
    while ((textNode = walker.nextNode())) {
      let value = textNode.nodeValue;
      for (const [source, target] of translations) value = value.replaceAll(source, target);
      textNode.nodeValue = value;
    }
    // Translate complete phrases after legacy labels so grammar does not depend on substring order.
    const setHTML = (selector, value) => {
      const element = document.querySelector(selector);
      if (element) element.innerHTML = value;
    };
    const guideBrand = document.querySelector(".brand");
    if (guideBrand) guideBrand.setAttribute("aria-label", "CSGClaw onboarding guide home");
    const guideTag = document.querySelector(".brand .tag");
    if (guideTag) guideTag.textContent = "Onboarding guide";
    document
      .querySelectorAll("#step-1 [data-en], #step-2 [data-en], #step-4 [data-en], #step-5 [data-en], #step-7 [data-en]")
      .forEach((element) => {
        element.textContent = element.getAttribute("data-en");
      });
    document
      .querySelector("#step-1 .new-task-mock")
      ?.setAttribute(
        "aria-label",
        "CSGClaw new task preview: choose Manager, describe the task, and select a folder and model",
      );
    document
      .querySelector("#step-2 .agents-settings-mock")
      ?.setAttribute("aria-label", "CSGClaw agents and teams preview");
    document
      .querySelector("#step-4 .skills-settings-mock")
      ?.setAttribute("aria-label", "CSGClaw skills settings preview");
    document
      .querySelector("#step-5 .schedule-dialog-mock")
      ?.setAttribute("aria-label", "CSGClaw new scheduled task dialog preview");
    document
      .querySelector("#step-7 .model-settings-mock")
      ?.setAttribute("aria-label", "CSGClaw model services settings preview");
    setHTML("#step-1 .copy h2", "Choose the right starting point:<br>tasks, team tasks, and scheduled tasks");
    setHTML(
      "#step-1 .points",
      "<li><span><b>Task</b>: Choose one agent, describe the work, and press Enter.</span></li><li><span><b>Team task</b>: Choose a team as the assignee. Manager breaks down the work and delegates it; you review the results.</span></li><li><span><b>Scheduled task</b>: Set a time and prompt to create a trackable task automatically.</span></li>",
    );
    setHTML(
      "#step-2 .points li:first-child span",
      "<b>Manager</b> is the general-purpose agent. Choose it when you are unsure who to assign.",
    );
    setHTML(
      "#step-2 .points",
      "<li><span><b>Manager</b> is the general-purpose agent. Choose it when you are unsure who to assign.</span></li><li><span><b>Specialized agents</b> divide work by role: product, engineering, testing, research, or writing.</span></li><li><span><b>Teams</b> let you save frequently used agents together and call on them next time.</span></li>",
    );
    setHTML(
      "#step-3 .lead",
      "Agents cannot read your mind. Specify the <strong>goal, scope, deliverable, and constraints</strong> for more consistent results. For longer tasks, ask the agent to repeat its plan before starting.",
    );
    setHTML(
      "#step-6 .lead",
      "Choose a <strong>team</strong> as the assignee when creating a task. Manager breaks down the work, members work in parallel, and the results are combined. You do not need to open five separate chats.",
    );
    setHTML(
      "#step-8 .lead",
      "CSGClaw runs locally, and agents read and write files in a <strong>real working directory</strong>. Choose a folder when creating a task, and your outputs stay in the project.",
    );
    setHTML(
      "#step-9 .points",
      "<li><span>Start with an official template or choose one from <b>My templates</b>.</span></li><li><span>Templates include instructions, skills, and <b>a read-only memory summary</b>.</span></li><li><span>Reuse team setups without assigning members again each time.</span></li>",
    );
    document.documentElement.lang = "en";
    document.title = "CSGClaw Onboarding Guide · Your AI team on one computer";
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute(
        "content",
        "A 10-step guide to tasks, team collaboration, scheduled tasks, agents, skills, connectors, and local workspaces in CSGClaw.",
      );
    document.getElementById("exitGuide")?.setAttribute("title", "Exit guide (Esc)");
    document.getElementById("prevBtn")?.setAttribute("aria-label", "Previous step");
    document.getElementById("nextBtn")?.setAttribute("aria-label", "Next step");
    document
      .querySelectorAll('[aria-label="步骤导航"]')
      .forEach((el) => el.setAttribute("aria-label", "Step navigation"));
    document
      .querySelectorAll(
        '[aria-label="第 1 步"], [aria-label="第 2 步"], [aria-label="第 3 步"], [aria-label="第 4 步"], [aria-label="第 5 步"], [aria-label="第 6 步"], [aria-label="第 7 步"], [aria-label="第 8 步"], [aria-label="第 9 步"], [aria-label="第 10 步"]',
      )
      .forEach((el) =>
        el.setAttribute("aria-label", el.getAttribute("aria-label").replace("第 ", "Step ").replace(" 步", "")),
      );
    document.querySelectorAll("img[alt]").forEach((el) => {
      const alt = el.getAttribute("alt");
      if (alt)
        el.setAttribute(
          "alt",
          alt
            .replace(
              "CSGClaw 新建任务界面：选择智能体、输入任务、选择目录与模型",
              "CSGClaw new task screen: choose an agent, enter a task, and select a folder and model",
            )
            .replace(
              "CSGClaw 智能体页面：智能体列表、团队与新建入口",
              "CSGClaw agents page: agent list, teams, and creation options",
            )
            .replace(
              "CSGClaw 设置中的技能页面：已安装技能与技能市场",
              "CSGClaw skills settings: installed skills and skills marketplace",
            )
            .replace(
              "CSGClaw 新建定时任务弹窗：标题、指派给、提示词、计划与日期",
              "CSGClaw scheduled task dialog: title, assignee, prompt, schedule, and date",
            )
            .replace(
              "CSGClaw 设置中的模型服务：默认模型与服务商列表",
              "CSGClaw model settings: default model and provider list",
            ),
        );
    });
  }
  const slides = Array.from(document.querySelectorAll(".slide"));
  const dotsWrap = document.getElementById("dots");
  const prevBtn = document.getElementById("prevBtn");
  const nextBtn = document.getElementById("nextBtn");
  const counterNow = document.getElementById("counterNow");
  const counterTotal = document.getElementById("counterTotal");
  const progress = document.getElementById("progress");
  const dockChapter = document.getElementById("dockChapter");
  const chapterBtns = Array.from(document.querySelectorAll(".chapters button"));
  const stepSlides = slides.filter((s) => s.dataset.step);
  const total = stepSlides.length;
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let current = 0;

  // 圆点：封面 + 10 个步骤
  const dotSlides = [slides[0], ...stepSlides];
  dotSlides.forEach((slide, i) => {
    const b = document.createElement("button");
    b.type = "button";
    b.setAttribute("role", "tab");
    b.setAttribute(
      "aria-label",
      i === 0 ? (locale === "en" ? "Cover" : "封面") : locale === "en" ? "Step " + i : "第 " + i + " 步",
    );
    b.addEventListener("click", () => go(i));
    dotsWrap.appendChild(b);
  });
  const dots = Array.from(dotsWrap.children);

  function go(i, opts) {
    i = Math.max(0, Math.min(dotSlides.length - 1, i));
    const behavior = reduce || (opts && opts.instant) ? "auto" : "smooth";
    const el = dotSlides[i];
    el.scrollIntoView({ behavior, block: "start" });
    setActive(i);
    try {
      if (history.replaceState)
        history.replaceState(null, "", i === 0 ? location.pathname + location.search : "#" + el.id);
    } catch {
      /* File previews may not allow history updates. */
    }
  }

  function setActive(i) {
    current = i;
    const slide = dotSlides[i];
    const step = slide.dataset.step ? Number(slide.dataset.step) : 0;
    slides.forEach((s) => s.classList.toggle("is-active", s === slide));

    dots.forEach((d, di) => {
      d.setAttribute("aria-current", di === i ? "true" : "false");
      d.tabIndex = di === i ? 0 : -1;
    });

    if (step === 0) {
      counterNow.textContent = locale === "en" ? "Start" : "开始";
      counterTotal.textContent = "/ " + total;
      dockChapter.textContent = locale === "en" ? "Cover · Before you begin" : "封面 · 开始之前";
      progress.style.width = "0%";
      chapterBtns.forEach((b) => b.removeAttribute("aria-current"));
    } else {
      counterNow.textContent = String(step).padStart(2, "0");
      counterTotal.textContent = "/ " + total;
      const basic = step <= 5;
      dockChapter.textContent =
        (locale === "en" ? (basic ? "Getting started" : "Next level") : basic ? "新手篇" : "进阶篇") +
        " · " +
        (basic ? step : step - 5) +
        " / 5";
      progress.style.width = (step / total) * 100 + "%";
      chapterBtns.forEach((b) => {
        if (b.dataset.goto === (basic ? "1" : "6")) b.setAttribute("aria-current", "true");
        else b.removeAttribute("aria-current");
      });
    }

    prevBtn.disabled = i === 0;
    nextBtn.disabled = i === dotSlides.length - 1;
  }

  prevBtn.addEventListener("click", () => go(current - 1));
  nextBtn.addEventListener("click", () => go(current + 1));
  document.getElementById("startGuide")?.addEventListener("click", () => go(1));
  document
    .querySelectorAll("[data-next]:not(#startGuide)")
    .forEach((b) => b.addEventListener("click", () => go(current + 1)));
  document.querySelectorAll("[data-goto]").forEach((b) => {
    b.addEventListener("click", () => {
      const n = Number(b.dataset.goto);
      go(n === 0 ? 0 : dotSlides.findIndex((s) => s.dataset.step === String(n)));
    });
  });

  // 退出导览：嵌入 App 时由宿主关闭；独立打开时尽量返回上一页
  let toastTimer = null;
  function showToast(msg) {
    const t = document.getElementById("toast");
    if (!t) return;
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => t.classList.remove("show"), 2400);
  }
  function exitGuide() {
    const evt = new CustomEvent("csgclaw:guide-exit", { cancelable: true });
    if (!window.dispatchEvent(evt)) return; // 宿主已接管
    if (window.parent && window.parent !== window) {
      try {
        window.parent.postMessage({ type: "csgclaw:guide-exit" }, window.location.origin);
      } catch {
        /* The host close button remains available. */
      }
      return;
    }
    if (window.opener) {
      try {
        window.close();
        return;
      } catch {
        /* Fall back to browser history. */
      }
    }
    if (history.length > 1) {
      history.back();
      return;
    }
    showToast(locale === "en" ? "You can close this tab now" : "可以关闭当前标签页了");
  }
  document.getElementById("exitGuide")?.addEventListener("click", exitGuide);
  document.querySelectorAll("[data-exit]").forEach((b) => b.addEventListener("click", exitGuide));

  window.addEventListener("message", (event) => {
    if (event.source !== window.parent || event.origin !== window.location.origin) return;
    if (window.parent !== window && event.data?.type === "csgclaw:guide-layout") {
      const { controlsWidth, exitWidth } = event.data;
      if (Number.isFinite(controlsWidth) && controlsWidth >= 0 && controlsWidth <= 300) {
        document.documentElement.style.setProperty("--guide-controls-width", `${controlsWidth}px`);
      }
      if (Number.isFinite(exitWidth) && exitWidth > 0 && exitWidth <= 400) {
        const exitButton = document.getElementById("exitGuide");
        if (exitButton?.style.visibility === "hidden") exitButton.style.width = `${exitWidth}px`;
      }
    }
    if (event.data?.type === "csgclaw:guide-focus") {
      document.getElementById("startGuide")?.focus();
    }
  });

  // 键盘
  document.addEventListener("keydown", (e) => {
    const tag = (e.target.tagName || "").toLowerCase();
    if (e.key === "Tab") {
      const focusable = Array.from(
        document.querySelectorAll('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'),
      ).filter((element) => element instanceof HTMLElement && element.offsetParent !== null);
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === document.body)) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
      return;
    }
    if (tag === "input" || tag === "textarea" || e.target.isContentEditable) return;
    if (["ArrowRight", "ArrowDown", "PageDown", " "].includes(e.key)) {
      e.preventDefault();
      go(current + 1);
    } else if (["ArrowLeft", "ArrowUp", "PageUp"].includes(e.key)) {
      e.preventDefault();
      go(current - 1);
    } else if (e.key === "Escape") {
      e.preventDefault();
      exitGuide();
    } else if (e.key === "Home") {
      e.preventDefault();
      go(0);
    } else if (e.key === "End") {
      e.preventDefault();
      go(dotSlides.length - 1);
    }
  });

  // 滚动观察：取距离视口中线最近的幻灯片
  let ticking = false;
  function onScroll() {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => {
      ticking = false;
      const mid = window.scrollY + window.innerHeight * 0.45;
      let best = 0,
        bestDist = Infinity;
      dotSlides.forEach((s, i) => {
        const rect = s.getBoundingClientRect();
        const top = window.scrollY + rect.top;
        const center = top + Math.min(rect.height, window.innerHeight) * 0.5;
        const d = Math.abs(center - mid);
        if (d < bestDist) {
          bestDist = d;
          best = i;
        }
      });
      if (best !== current) setActive(best);
    });
  }
  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll);

  // 首屏定位：等布局稳定后再跳转
  if ("scrollRestoration" in history) history.scrollRestoration = "manual";
  window.scrollTo(0, 0);
  setActive(0);
  const hash = decodeURIComponent(location.hash.replace("#", ""));
  const target = dotSlides.findIndex((s) => s.id === hash);
  if (target > 0) {
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        go(target, { instant: true });
      }),
    );
  }

  // 触屏左右滑动
  let touchX = null,
    touchY = null;
  window.addEventListener(
    "touchstart",
    (e) => {
      touchX = e.touches[0].clientX;
      touchY = e.touches[0].clientY;
    },
    { passive: true },
  );
  window.addEventListener(
    "touchend",
    (e) => {
      if (touchX == null) return;
      const dx = e.changedTouches[0].clientX - touchX;
      const dy = e.changedTouches[0].clientY - touchY;
      if (Math.abs(dx) > 70 && Math.abs(dx) > Math.abs(dy) * 1.6) {
        go(current + (dx < 0 ? 1 : -1));
      }
      touchX = touchY = null;
    },
    { passive: true },
  );
})();
