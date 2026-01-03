# Clarity-AI

Clarity-AI is an **AI-driven learning platform** that turns YouTube video content into concise summaries, structured key points, and multi-level practice questions — all delivered through a sleek, modern mobile experience. This repository contains the **iOS client** built with SwiftUI, designed with production-level architecture and modern Apple design principles.

---

## 🚀 What Clarity-AI Does

Clarity-AI helps learners:
- **Summarize any YouTube video** into clear, easy-to-digest content
- **Generate automatic questions** at multiple difficulty levels for deeper understanding
- **Extract key concepts and takeaways** to guide study and revision
- **Switch between advanced AI models** via subscription plans

Clarity-AI is ideal for students, professionals, and lifelong learners who want to *learn smarter and faster* from video content.

---

## 📱 iOS App Highlights

The iOS client brings Clarity-AI to life with:

### Modern Apple Design
- **SwiftUI-first UI** using Apple’s latest design patterns
- **Clean, production-grade folder structure**
- **Componentized and scalable architecture**
- **Seamless, fast UI interactions** with smooth animations and transitions
- **Adaptive layouts**, accessibility support, and polished UX

### Performance & Quality
- **Asynchronous and responsive UI** using structured concurrency (`async/await`)
- **Efficient networking and caching**
- Clean separation between views, models, and services
- Designed for maintainability and scalability

---

## 🧠 Key Features

- Video summarization with AI-generated insights
- Multi-difficulty level quiz questions
- Fast and fluid UI with SwiftUI and Combine/async
- Modular and testable application layers
- Premium model selector for dynamic AI quality

---

## ⚙️ Full Tech Stack

### Frontend (Mobile)
- **Swift & SwiftUI** – Rendering UI using declarative interface
- **Combine / Async-Await** – Handling asynchronous data streams
- **MVVM & Clean Architecture** – Structured for testability and scalability
- Custom API layer for backend integration

### Backend (Powered by Go Microservices)
- **Golang microservice architecture** with well-defined services
- **gRPC APIs** for fast, efficient communication between services
- **Rate limiting & service governance** for stable and robust request handling
- **Docker** containers for each service (`user-service`, `video-service`, etc.)
- **Database migrations and schema versioning**
- Modular design using proto definitions (`/api/proto`)
- Scalable deployment composition using `docker-compose.yml`

These backend services power the AI summarization, question generation, and model orchestration.

---

## 🧠 How It Works

1. User pastes a **YouTube video link**
2. SwiftUI app sends request to the backend
3. Backend services:
   - Fetch transcript and metadata
   - Run AI models to generate a summary
   - Produce quiz questions and structured map of concepts
4. iOS client renders the results in a polished, interactive UI

---

## 🚀 Getting Started

### Clone the Repository

```bash
git clone https://github.com/JAAT-1628/Clarity-ai-client.git
cd Clarity-ai-client
