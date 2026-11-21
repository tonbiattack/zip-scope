<script setup lang="ts">
// A small modal used for rename and move prompts, and for destructive
// confirmations. Keeping one component means every dialog behaves the same.
import { nextTick, onMounted, ref } from 'vue'

const props = defineProps<{
  title: string
  message?: string
  // value is the initial text for a prompt; omit for a confirmation.
  value?: string
  placeholder?: string
  confirmLabel?: string
  danger?: boolean
  // prompt shows a text field; otherwise only the buttons are shown.
  prompt?: boolean
}>()

const emit = defineEmits<{
  (e: 'confirm', value: string): void
  (e: 'cancel'): void
}>()

const text = ref(props.value ?? '')
const input = ref<HTMLInputElement | null>(null)
const error = ref<string | null>(null)

onMounted(async () => {
  if (props.prompt) {
    await nextTick()
    input.value?.focus()
    input.value?.select()
  }
})

function confirm() {
  if (props.prompt) {
    const v = text.value.trim()
    if (!v) {
      error.value = '名前を入力してください'
      return
    }
    emit('confirm', v)
    return
  }
  emit('confirm', '')
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('cancel')
  } else if (e.key === 'Enter') {
    e.preventDefault()
    confirm()
  }
}
</script>

<template>
  <div class="backdrop" @click.self="emit('cancel')" @keydown="onKey">
    <div class="dialog" role="dialog" aria-modal="true">
      <h2>{{ title }}</h2>
      <p v-if="message" class="message">{{ message }}</p>
      <input
        v-if="prompt"
        ref="input"
        v-model="text"
        class="field"
        type="text"
        :placeholder="placeholder"
        @keydown="onKey"
      />
      <p v-if="error" class="error">{{ error }}</p>
      <div class="actions">
        <button class="btn" @click="emit('cancel')">キャンセル</button>
        <button :class="['btn', danger ? 'danger' : 'primary']" @click="confirm">
          {{ confirmLabel ?? 'OK' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 90;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
}
.dialog {
  width: min(420px, 92vw);
  background: var(--panel);
  border: 1px solid var(--border-strong);
  border-radius: 10px;
  padding: 18px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.55);
}
h2 {
  font-size: 14px;
  margin: 0 0 10px;
}
.message {
  font-size: 12px;
  color: var(--fg-dim);
  margin: 0 0 12px;
  line-height: 1.6;
  word-break: break-all;
}
.field {
  width: 100%;
  padding: 7px 9px;
  border-radius: 6px;
  border: 1px solid var(--border-strong);
  background: var(--bg);
  color: var(--fg);
  font-size: 13px;
  box-sizing: border-box;
}
.error {
  color: var(--danger);
  font-size: 11px;
  margin: 6px 0 0;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}
</style>
