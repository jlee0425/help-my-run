import React, { useState } from 'react';
import {
  View, Text, FlatList, TextInput, Pressable, StyleSheet,
} from 'react-native';
import { useChatHistory, useSendChat, useClearChat } from '../src/api/hooks';
import type { ChatMessage } from '../src/api/types';

function Bubble({ msg }: { msg: ChatMessage }) {
  const isUser = msg.role === 'user';
  return (
    <View
      testID={isUser ? 'bubble-user' : 'bubble-assistant'}
      style={[styles.bubble, isUser ? styles.bubbleUser : styles.bubbleAssistant]}
    >
      <Text style={isUser ? styles.bubbleUserText : styles.bubbleAssistantText}>
        {msg.content}
      </Text>
    </View>
  );
}

export default function ChatScreen() {
  const chat = useChatHistory();
  const send = useSendChat();
  const clear = useClearChat();
  const [text, setText] = useState('');

  const messages = chat.data?.messages ?? [];

  const onSend = () => {
    const message = text.trim();
    if (!message || send.isPending) return;
    send.mutate({ message });
    setText('');
  };

  return (
    <View style={styles.container}>
      <FlatList
        style={styles.list}
        contentContainerStyle={styles.listContent}
        data={messages}
        keyExtractor={(_, i) => String(i)}
        ListEmptyComponent={
          chat.isPending ? null : (
            <Text testID="chat-empty" style={styles.empty}>
              Ask a question about your training data.
            </Text>
          )
        }
        renderItem={({ item }: { item: ChatMessage }) => <Bubble msg={item} />}
      />

      {send.isPending ? (
        <Text testID="chat-loading" style={styles.loading}>
          Thinking…
        </Text>
      ) : null}

      {send.isError ? (
        <Text testID="chat-error" style={styles.error}>
          {(send.error as Error)?.message ?? 'Something went wrong. Try again.'}
        </Text>
      ) : null}

      <View style={styles.inputRow}>
        <TextInput
          testID="chat-input"
          style={styles.input}
          value={text}
          onChangeText={setText}
          placeholder="Ask about your training…"
          multiline
        />
        <Pressable
          testID="btn-send-chat"
          style={styles.sendButton}
          disabled={send.isPending}
          onPress={onSend}
        >
          <Text style={styles.sendButtonText}>{send.isPending ? '…' : 'Send'}</Text>
        </Pressable>
      </View>

      <Pressable
        testID="btn-clear-chat"
        style={styles.clearButton}
        disabled={clear.isPending}
        onPress={() => clear.mutate()}
      >
        <Text style={styles.clearButtonText}>
          {clear.isPending ? 'Clearing…' : 'Clear chat'}
        </Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, padding: 16, gap: 8 },
  list: { flex: 1 },
  listContent: { gap: 8, paddingVertical: 8 },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8, textAlign: 'center' },
  bubble: { maxWidth: '85%', borderRadius: 12, paddingHorizontal: 12, paddingVertical: 8 },
  bubbleUser: { alignSelf: 'flex-end', backgroundColor: '#fc4c02' },
  bubbleAssistant: { alignSelf: 'flex-start', backgroundColor: '#eee' },
  bubbleUserText: { color: '#fff', fontSize: 15 },
  bubbleAssistantText: { color: '#222', fontSize: 15 },
  loading: { fontSize: 13, color: '#666', fontStyle: 'italic', paddingVertical: 4 },
  error: { fontSize: 13, color: '#c0392b', paddingVertical: 4 },
  inputRow: { flexDirection: 'row', alignItems: 'flex-end', gap: 8 },
  input: {
    flex: 1, borderWidth: StyleSheet.hairlineWidth, borderColor: '#ddd',
    borderRadius: 8, paddingHorizontal: 10, paddingVertical: 8, fontSize: 15,
    maxHeight: 120, backgroundColor: '#fff',
  },
  sendButton: {
    backgroundColor: '#fc4c02', borderRadius: 8, paddingHorizontal: 16,
    paddingVertical: 10, alignItems: 'center', justifyContent: 'center',
  },
  sendButtonText: { color: '#fff', fontSize: 15, fontWeight: '600' },
  clearButton: {
    alignSelf: 'flex-start', borderWidth: StyleSheet.hairlineWidth,
    borderColor: '#fc4c02', borderRadius: 8, paddingHorizontal: 12, paddingVertical: 8,
  },
  clearButtonText: { fontSize: 14, color: '#fc4c02', fontWeight: '600' },
});
