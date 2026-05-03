// Developed by Taipei Urban Intelligence Center 2023-2024

import { defineStore } from "pinia";

const VALID_THEMES = ["dark", "light"];

export const useThemeStore = defineStore("theme", {
	state: () => ({
		theme: "dark",
	}),
	actions: {
		setTheme(theme) {
			const nextTheme = VALID_THEMES.includes(theme) ? theme : "dark";
			this.theme = nextTheme;

			if (typeof document !== "undefined") {
				document.documentElement.classList.toggle(
					"light-mode",
					nextTheme === "light"
				);
			}

			try {
				localStorage.setItem("theme", nextTheme);
			} catch {
				// localStorage may be blocked in privacy modes.
			}
		},
		initTheme() {
			let initialTheme = "dark";
			try {
				const savedTheme = localStorage.getItem("theme");
				initialTheme = VALID_THEMES.includes(savedTheme)
					? savedTheme
					: "dark";
			} catch {
				initialTheme = "dark";
			}

			this.setTheme(initialTheme);
		},
	},
});
