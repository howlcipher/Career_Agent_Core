import requests
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import concurrent.futures


app = FastAPI(title="Career Agent NLP Microservice")


class JobApplicationRequest(BaseModel):
    job_title: str
    job_description: str
    parsed_document: str
    tone_context: str = ""
    comp_context: str = ""
    provider: str = "ollama"
    api_key: str = ""
    ollama_host: str = "http://localhost:11434"
    model: str = "llama3"


class JobApplicationResponse(BaseModel):
    resume: str
    cover_letter: str
    interview_prep: str


def call_ollama(
    prompt: str, system: str, host: str, model: str, temperature: float = -1
) -> str:
    payload = {
        "model": model,
        "prompt": prompt,
        "system": system,
        "stream": False,
        "keep_alive": "30m"
    }
    if temperature >= 0:
        payload["options"] = {"temperature": temperature}

    try:
        resp = requests.post(
            f"{host}/api/generate", json=payload, timeout=300
        )
        resp.raise_for_status()
        data = resp.json()
        return data.get("response", "").strip()
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"Ollama generation failed: {str(e)}"
        )


@app.post("/process", response_model=JobApplicationResponse)
def process_job_application(req: JobApplicationRequest):
    gap_sys = (
        "You are an expert technical recruiter. Analyze the job description "
        "and the candidate's profile. Identify key skills, tools, or "
        "requirements in the job description that are MISSING or "
        "UNDERREPRESENTED in the candidate's profile. Output ONLY a "
        "comma-separated list of these missing keywords. If none, "
        "output 'NONE'."
    )
    gap_prompt = (
        f"Job Title: {req.job_title}\n\nJob Description: "
        f"{req.job_description}\n\nMy Background:\n{req.parsed_document}\n\n"
        "Missing keywords:"
    )

    gap_out = call_ollama(
        gap_prompt, gap_sys, req.ollama_host, req.model, temperature=-1
    )

    parsed_doc = req.parsed_document
    if gap_out.upper() != "NONE" and gap_out:
        parsed_doc += (
            "\n\nNOTE: The following keywords from the job description are "
            "missing in the base profile. Find creative but truthful ways "
            f"to address them if possible: {gap_out}"
        )

    sys_resume = (
        "You are an expert technical recruiter. Analyze the job description "
        "and tailor the base resume. Emphasize Python and Go automation, "
        "log parsing, MS Cyber Defense, and secure networking. Use the "
        "heading Executive Summary. Do not hallucinate metrics. "
        "Do not use hyphens."
    )
    prompt_resume = (
        f"Job Title: {req.job_title}\n\nJob Description: "
        f"{req.job_description}\n\nMy Background:\n{parsed_doc}\n\nPlease "
        "output the tailored Markdown resume based on my profile. Output ONLY "
        "the markdown resume without extra commentary."
    )

    sys_cover = (
        "You are an expert technical recruiter. Analyze the job description "
        "and tailor the cover letter. Emphasize Python and Go automation, "
        "log parsing, MS Cyber Defense, and secure networking. Write a "
        "three paragraph cover letter highlighting 9 plus years experience. "
        "Do not use hyphens."
    )
    prompt_cover = (
        f"Job Title: {req.job_title}\n\nJob Description: "
        f"{req.job_description}\n\nMy Background:\n{parsed_doc}"
        f"{req.tone_context}{req.comp_context}\n\nPlease output a tailored "
        "plain text cover letter. Output ONLY the cover letter."
    )

    sys_prep = (
        "You are an expert technical recruiter. Analyze the job description "
        "and create an interview preparation sheet."
    )
    prompt_prep = (
        f"Job Title: {req.job_title}\n\nJob Description: "
        f"{req.job_description}\n\nMy Background:\n{parsed_doc}\n\nPlease "
        "output a cheat sheet of likely interview questions. Output ONLY "
        "the cheat sheet."
    )

    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as executor:
        f_resume = executor.submit(
            call_ollama, prompt_resume, sys_resume, req.ollama_host, req.model
        )
        f_cover = executor.submit(
            call_ollama, prompt_cover, sys_cover, req.ollama_host, req.model
        )
        f_prep = executor.submit(
            call_ollama, prompt_prep, sys_prep, req.ollama_host, req.model
        )

        resume_out = f_resume.result()
        cover_out = f_cover.result()
        prep_out = f_prep.result()

    return JobApplicationResponse(
        resume=resume_out,
        cover_letter=cover_out,
        interview_prep=prep_out
    )
