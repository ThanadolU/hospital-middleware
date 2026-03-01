# 🏥 Hospital Management Backend API

A containerized backend API for managing hospital staff authentication and patient search, built with Go, Gin, PostgreSQL, Docker, and Nginx.

## 📌 Project Overview

This project implements a layered backend architecture for a hospital system.
It supports:

- Staff registration and authentication (JWT-based)
- Protected patient search API
- Multi-hospital data structure
- Dockerized deployment
- Nginx reverse proxy

**Architecture:**

```
Client → Nginx → Go (Gin) API → PostgreSQL
```

## 🏗️ Tech Stack

- Go (Gin framework)
- GORM
- PostgreSQL
- Docker & Docker Compose
- Nginx (Reverse Proxy)
- JWT Authentication

## 📁 Project Structure

```
hospital-middleware/
│
├── cmd/
│   └── main.go              # Application entry point
├── config/                  # DB connection & seed logic
├── internal/
│   ├── handler/             # HTTP handlers
│   ├── middleware/          # JWT middleware
│   ├── models/              # GORM models
│   ├── repository/          # Data access layer
│   ├── routes/              # Route registration
│   └── service/             # Business logic
├── nginx/
│   └── nginx.conf           # Reverse proxy configuration
├── pkg/
│   └── utils/               # Utility functions
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 🧱 Architecture Pattern

Layered architecture:

```
Route
  ↓
Handler (HTTP Layer)
  ↓
Service (Business Logic)
  ↓
Repository (Database Access)
  ↓
PostgreSQL
```

**Advantages:**

- Separation of concerns
- Maintainable structure
- Scalable design
- Testable components

## 🗄️ Database Design

**Entities:**

- Hospital
- Staff
- Patient

**Relationships:**

- One Hospital → Many Staff
- One Hospital → Many Patients

Data ownership is enforced using foreign keys (hospital_id).

## 🔐 Authentication

- JWT-based authentication
- Protected routes require:
  - `Authorization: Bearer <token>`
- Passwords are hashed before storage

## 🚀 Running the Project

### 1️⃣ Clone Repository

```bash
git clone https://github.com/your-username/hospital-middleware.git
cd hospital-middleware
```

### 2️⃣ Run with Docker

```bash
docker-compose up --build
```

### 3️⃣ Access API

**Base URL:**

```
http://localhost:8081/api
```

**Health Check:**

```
GET http://localhost:8081/api/health
```

## 📡 API Endpoints

### Health Check

**GET** `/api/health`

**Response:**

```json
{
  "status": "ok"
}
```

### Authentication

**Base Path:** `/api/auth`

#### Register

**POST** `/api/auth/register`

**Request:**

```json
{
  "email": "staff@example.com",
  "password": "password123",
  "hospital_id": 1
}
```

#### Login

**POST** `/api/auth/login`

**Request:**

```json
{
  "email": "staff@example.com",
  "password": "password123",
  "hospital_id": 1
}
```

**Response:**

```json
{
  "token": "jwt_token_here"
}
```

### Patient Search (Protected)

Search patients using optional query parameters.

**GET** `/api/patients/search`

#### Query Parameters

All parameters are optional. Results are filtered dynamically based on provided fields.

| Parameter        | Type   | Description                          |
|------------------|--------|--------------------------------------|
| national_id      | string | National ID number                   |
| passport_id      | string | Passport ID                          |
| first_name       | string | Thai first name                      |
| middle_name      | string | Thai middle name                     |
| last_name        | string | Thai last name                       |
| date_of_birth    | string | Format: YYYY-MM-DD                   |
| phone_number     | string | Phone number                         |
| email            | string | Email address                        |


**Example:**

```
GET /api/patients/search?first_name=John
```

**Headers required:**

```
Authorization: Bearer <JWT_TOKEN>
```

**Example Response:**
```
{
  "data": [
    {
      "id": "b3f2ae54-a60c-45bc-bb2c-60f9f335f1ee",
      "hospital_id": "bc885924-0fe7-4838-8f4f-18bd85cb3e8a",
      "hospital": {
        "id": "bc885924-0fe7-4838-8f4f-18bd85cb3e8a",
        "name": "Hospital A",
        "created_at": "2026-02-28T09:24:02.787786Z",
        "updated_at": "2026-02-28T09:24:02.787786Z"
      },
      "first_name_th": "",
      "middle_name_th": "",
      "last_name_th": "",
      "first_name_en": "John",
      "middle_name_en": "Michael",
      "last_name_en": "Doe",
      "date_of_birth": "1995-02-20T00:00:00Z",
      "patient_hn": "HN0003",
      "national_id": "",
      "passport_id": "AA1234567",
      "phone_number": "0869876543",
      "email": "john.doe@example.com",
      "gender": "M",
      "created_at": "2026-02-28T09:24:02.804776Z",
      "updated_at": "2026-02-28T09:24:02.804776Z"
    },
  ],
  "success": true
}
```

## 🧪 Initial Data Seeding

- Database auto-migrates on startup
- Seed data runs only if database is empty
- Creates:
  - Default Hospital
  - Sample Patients

## 🐳 Deployment Notes

- Nginx acts as reverse proxy
- Go app runs on port 8000
- Nginx exposes port 8081
- PostgreSQL runs in separate container

## 📌 Future Improvements

- API versioning (`/api/v1`)
- Role-based access control
- Pagination for patient search
- Proper migration tool integration
- Unit tests

## 👤 Author

Thanadol Udomsirinanchai
