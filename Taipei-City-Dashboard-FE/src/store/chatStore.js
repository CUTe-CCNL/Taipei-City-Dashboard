import { nextTick, ref, watch } from "vue";
import { defineStore } from "pinia";
import http from "../router/axios";
import { useAuthStore } from "./authStore";

const ASSISTANT_ENDPOINT = "/ai/chat/twai";
const TYPEWRITER_TICK_MS = 8;

export const useChatStore = defineStore("chat", () => {
	// 預設訊息
	const defaultChatData = [
		{
			id: 1,
			role: "bot",
			isDefault: true,
			content:
				"您好，我是【臺北城市儀表板】市政小助理，很高興為您服務！\n您可以直接提問市政資料問題（例如：上週垃圾收運量是否異常），也可以請我推薦可用圖表。\n\n📩 聯絡信箱：tuic@gov.taipei\n🏢 臺北大數據中心\n",
		},
	];

	const recommendComponents = ref(null);
	const useAiAssistant = ref(true);
	const aiStreaming = ref(false);
	const toolLoading = ref(false);

	// 從 sessionStorage 讀取
	const savedChatData = JSON.parse(sessionStorage.getItem("chatData")) || [];

	// 拼接預設訊息 + sessionStorage 的聊天紀錄
	const chatData = ref([...defaultChatData, ...savedChatData]);
	let persistTimer = null;

	const persistChatDataToSession = () => {
		const userBotMessages = chatData.value.filter((item) => !item.isDefault);
		sessionStorage.setItem("chatData", JSON.stringify(userBotMessages));
	};

	// 監聽 chatData 的變化，自動同步到 sessionStorage
	watch(
		chatData,
		() => {
			if (aiStreaming.value) {
				return;
			}
			if (persistTimer) {
				clearTimeout(persistTimer);
			}
			persistTimer = setTimeout(persistChatDataToSession, 0);
		},
		{ deep: true },
	);

	const addChatData = (newChatData) => {
		const message = {
			id: chatData.value.length + 1,
			isDefault: false,
			...newChatData,
		};
		chatData.value.push(message);
		// Return the reactive entry from the store array so incremental
		// mutations (SSE chunks) trigger Vue updates immediately.
		return chatData.value[chatData.value.length - 1];
	};

	const normalizeRecommendedComponents = (items = []) => {
		return Array.from(
			items
				.reduce((map, item) => {
					const key = item.index;
					const existing = map.get(key);
					if (!existing) {
						map.set(key, item);
						return map;
					}
					if (item.city === "metrotaipei") {
						map.set(key, item);
						return map;
					}
					if ((item.score || 0) > (existing.score || 0)) {
						map.set(key, item);
					}
					return map;
				}, new Map())
				.values(),
		);
	};

	const extractTextChunk = (payload) => {
		if (!payload) return "";
		if (typeof payload === "string") return payload;
		if (payload.generated_text) return payload.generated_text;
		if (
			Array.isArray(payload.choices) &&
			payload.choices[0]?.delta?.content
		) {
			return payload.choices[0].delta.content;
		}
		return "";
	};

	const getAuthToken = () => {
		const authStore = useAuthStore();
		return authStore.token || localStorage.getItem("token") || "";
	};

	const attachComponentsToMessage = (message, components) => {
		if (!components || components.length === 0) return;
		const topK = [...components].sort((a, b) => (b.score || 0) - (a.score || 0));
		message.relations = topK;
		message.button = [{ id: 1, text: "建立儀表板" }];
	};

	const streamAiChat = async (question) => {
		const trimmedQuestion = question.trim();
		if (!trimmedQuestion) {
			return { ok: false, reason: "empty_question" };
		}
		if (aiStreaming.value) {
			return { ok: false, reason: "busy" };
		}

		if (!useAiAssistant.value) {
			aiStreaming.value = true;
			try {
				await addQueryData({ role: "user", content: trimmedQuestion });
				return { ok: true, fallback: true };
			} finally {
				aiStreaming.value = false;
			}
		}
		aiStreaming.value = true;

		addChatData({
			role: "user",
			content: trimmedQuestion,
		});

		recommendComponents.value = [];
		const botMessage = addChatData({
			role: "bot",
			content: "",
			relations: null,
		});

		toolLoading.value = true;
		let responseText = "";
		let recommendedFromEvent = [];
		let typingQueue = [];
		let typingRunning = false;
		let typingDrainPromise = Promise.resolve();

		const sleep = (ms) =>
			new Promise((resolve) => {
				setTimeout(resolve, ms);
			});
		const waitForPaint = () =>
			new Promise((resolve) => {
				requestAnimationFrame(() => resolve());
			});

		const runTypewriter = async () => {
			if (typingRunning) return;
			typingRunning = true;
			try {
				while (typingQueue.length > 0) {
					const nextChar = typingQueue.shift();
					responseText += nextChar;
					botMessage.content = responseText;
					await nextTick();
					await waitForPaint();
					await sleep(TYPEWRITER_TICK_MS);
				}
			} finally {
				typingRunning = false;
			}
		};

		const enqueueTypewriterText = (text) => {
			if (!text) return;
			typingQueue.push(...Array.from(text));
			typingDrainPromise = typingDrainPromise.then(runTypewriter);
		};

		try {
			const response = await fetch(
				`${import.meta.env.VITE_API_URL}${ASSISTANT_ENDPOINT}`,
				{
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						Authorization: `Bearer ${getAuthToken()}`,
					},
					body: JSON.stringify({
						stream: true,
						session: "",
						messages: [
							{
								role: "user",
								content: trimmedQuestion,
							},
						],
					}),
				},
			);

			if (!response.ok || !response.body) {
				throw new Error(`SSE request failed with status ${response.status}`);
			}

			const reader = response.body.getReader();
			const decoder = new TextDecoder("utf-8");
			let buffer = "";
			let currentEvent = "message";
			let currentDataLines = [];

			const handleSseEvent = (event, data) => {
				if (!data || data === "[DONE]") return;

				if (event === "components") {
					try {
						const payload = JSON.parse(data);
						const source = Array.isArray(payload?.components)
							? payload.components
							: [];
						recommendedFromEvent = normalizeRecommendedComponents(source);
					} catch (error) {
						console.error("components event parse error:", error);
					}
					return;
				}

				try {
					const payload = JSON.parse(data);
					const chunkText = extractTextChunk(payload);
					if (chunkText) {
						enqueueTypewriterText(chunkText);
					}
				} catch {
					enqueueTypewriterText(data);
				}
			};

			const flushSseEvent = () => {
				if (currentDataLines.length === 0) {
					currentEvent = "message";
					return;
				}

				const data = currentDataLines.join("\n");
				const event = currentEvent || "message";
				currentDataLines = [];
				currentEvent = "message";
				handleSseEvent(event, data);
			};

			for (;;) {
				const { done, value } = await reader.read();
				if (done) break;

				buffer += decoder.decode(value, { stream: true });
				let lineEnd = buffer.indexOf("\n");
				while (lineEnd !== -1) {
					let line = buffer.slice(0, lineEnd);
					buffer = buffer.slice(lineEnd + 1);
					if (line.endsWith("\r")) {
						line = line.slice(0, -1);
					}
					lineEnd = buffer.indexOf("\n");

					if (line === "") {
						flushSseEvent();
						continue;
					}
					if (line.startsWith(":")) {
						continue;
					}
					if (line.startsWith("event:")) {
						currentEvent = line.slice(6).trim() || "message";
						continue;
					}
					if (line.startsWith("data:")) {
						currentDataLines.push(line.slice(5).trim());
						// Tolerate malformed streams that miss blank-line separators:
						// flush immediately for token-by-token UX.
						flushSseEvent();
						continue;
					}

					// Fallback for plain text lines.
					currentDataLines.push(line);
					flushSseEvent();
				}
			}

			const remaining = buffer.trim();
			if (remaining) {
				if (remaining.startsWith("data:")) {
					currentDataLines.push(remaining.slice(5).trim());
				} else if (!remaining.startsWith("event:") && !remaining.startsWith(":")) {
					currentDataLines.push(remaining);
				}
			}
			flushSseEvent();
			await typingDrainPromise;

			recommendComponents.value = normalizeRecommendedComponents(recommendedFromEvent);
			attachComponentsToMessage(botMessage, recommendComponents.value);

			if (!botMessage.content?.trim()) {
				botMessage.content = "查詢完成。若您想看相關圖表，我可以再幫您推薦。";
			}

			await saveChatLog(trimmedQuestion, {
				text: botMessage.content,
				components: recommendComponents.value,
			});

			return { ok: true };
		} catch (error) {
			console.error("streamAiChat error:", error);
			if (!botMessage.content?.trim()) {
				botMessage.content =
					"很抱歉，目前無法完成資料查詢。請稍後再試，或先改用關鍵字查詢組件。";
			}
			return { ok: false, error };
		} finally {
			toolLoading.value = false;
			aiStreaming.value = false;
			persistChatDataToSession();
		}
	};

	const addQueryData = async (newChatData) => {
		addChatData(newChatData);

		recommendComponents.value = [];

		try {
			const response = await http.post(
				"/vector/component",
				new URLSearchParams({
					query: newChatData.content,
					limit: 10,
					score: 0.8,
				}),
				{
					headers: {
						"Content-Type": "application/x-www-form-urlencoded",
					},
				},
			);

			if (response.data?.data?.length > 0) {
				recommendComponents.value = normalizeRecommendedComponents(response.data.data);
			}
		} catch (error) {
			console.error("VectorAnalysisError :", error);
		}

		if (recommendComponents.value && recommendComponents.value.length > 0) {
			const topK = [...recommendComponents.value].sort(
				(a, b) => (b.score || 0) - (a.score || 0),
			);
			addChatData({
				role: "bot",
				button: [{ id: 1, text: "建立儀表板" }],
				content:
					"您好 😊\n以下是根據您的問題推薦的「組件清單」，可直接建立個人儀表板。",
				relations: topK,
			});
		} else {
			addChatData({
				role: "bot",
				content: "很抱歉，您提供的描述沒有相似組件，請再試試其他關鍵字。",
			});
		}

		await saveChatLog(newChatData.content, recommendComponents.value);
	};

	const saveChatLog = async (question, answer) => {
		try {
			const formData = new FormData();
			const d = new Date();
			const todayId =
				d.getFullYear() +
				String(d.getMonth() + 1).padStart(2, "0") +
				String(d.getDate()).padStart(2, "0");

			formData.append("session", "session_" + todayId);
			formData.append("question", question);
			formData.append("answer", JSON.stringify(answer));

			await http.post("/chatlog/", formData, {
				headers: {
					"Content-Type": "multipart/form-data",
				},
			});
		} catch (error) {
			console.error("saveChatLog error:", error);
		}
	};

	return {
		chatData,
		recommendComponents,
		useAiAssistant,
		aiStreaming,
		toolLoading,
		addChatData,
		addQueryData,
		streamAiChat,
		saveChatLog,
	};
});
