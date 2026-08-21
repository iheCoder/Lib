const CHATGPT_HOME_URL = "https://chatgpt.com/";

/**
 * Replace the extension page in history so pressing Back does not create a
 * loop between ChatGPT and the overridden Chrome new-tab page.
 */
function openChatGptHome() {
  window.location.replace(CHATGPT_HOME_URL);
}

openChatGptHome();
